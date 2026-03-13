#include "market_service.hpp"
#include "market.pb.h"
#include <algorithm>
#include <cstring>
#include <iostream>
#include <stdexcept>

// ── Constructor ──────────────────────────────────────────────────────────────

MarketService::MarketService(const ServiceConfig& cfg) : cfg_(cfg) {
    if (cfg_.test_publisher) return;  // test mode: skip Kafka

    std::string errstr;

    auto* cconf = RdKafka::Conf::create(RdKafka::Conf::CONF_GLOBAL);
    cconf->set("bootstrap.servers", cfg_.brokers, errstr);
    cconf->set("group.id",          cfg_.group_id, errstr);
    cconf->set("auto.offset.reset", "earliest",    errstr);
    consumer_.reset(RdKafka::KafkaConsumer::create(cconf, errstr));
    delete cconf;
    if (!consumer_) throw std::runtime_error("KafkaConsumer create failed: " + errstr);
    consumer_->subscribe({cfg_.in_topic});

    auto* pconf = RdKafka::Conf::create(RdKafka::Conf::CONF_GLOBAL);
    pconf->set("bootstrap.servers", cfg_.brokers, errstr);
    producer_.reset(RdKafka::Producer::create(pconf, errstr));
    delete pconf;
    if (!producer_) throw std::runtime_error("Producer create failed: " + errstr);
}

// ── Lifecycle ────────────────────────────────────────────────────────────────

void MarketService::run() {
    running_ = true;
    while (running_) {
        std::unique_ptr<RdKafka::Message> msg(
            consumer_->consume(cfg_.poll_timeout_ms));
        if (!msg) continue;
        if (msg->err() == RdKafka::ERR__TIMED_OUT) continue;
        if (msg->err() != RdKafka::ERR_NO_ERROR) {
            std::cerr << "Kafka consume error: " << msg->errstr() << '\n';
            continue;
        }

        std::string symbol(static_cast<const char*>(msg->key_pointer()),
                           msg->key_len());
        secwager::MarketCommand cmd;
        if (!cmd.ParseFromArray(msg->payload(), static_cast<int>(msg->len()))) {
            std::cerr << "Failed to parse MarketCommand (symbol=" << symbol << ")\n";
            continue;
        }
        process(symbol, cmd);
        producer_->poll(0);  // non-blocking delivery-report drain
    }
    consumer_->close();
    producer_->flush(10'000);
}

void MarketService::stop() {
    running_ = false;
}

// ── Command dispatch ─────────────────────────────────────────────────────────

void MarketService::process(const std::string& symbol,
                             const secwager::MarketCommand& cmd) {
    switch (cmd.command_case()) {
        case secwager::MarketCommand::kNewOrder:
            handle_new_order(symbol, cmd.new_order()); break;
        case secwager::MarketCommand::kCancel:
            handle_cancel(symbol, cmd.cancel()); break;
        default:
            std::cerr << "Unknown command case for symbol=" << symbol << '\n';
    }
}

// ── Engine registry ──────────────────────────────────────────────────────────

MatchEngine& MarketService::engine_for(const std::string& symbol) {
    auto [it, inserted] = engines_.try_emplace(symbol, nullptr);
    if (inserted) {
        std::string sym = it->first;  // stable map key; safe after rehash
        it->second = std::make_unique<MatchEngine>(
            [this, sym](const Execution& e) {
                secwager::Execution rpt;
                rpt.set_symbol(sym);
                rpt.set_buyer (std::string(e.buyer,  strnlen(e.buyer,  8)));
                rpt.set_seller(std::string(e.seller, strnlen(e.seller, 8)));
                rpt.set_price(e.price);
                rpt.set_size(e.size);
                publish(cfg_.exec_topic, sym, rpt);

                if (fill_accumulator_) *fill_accumulator_ += e.size;
            });
    }
    return *it->second;
}

// ── Helpers ──────────────────────────────────────────────────────────────────

MarketService::TopOfBook MarketService::snapshot_tob(MatchEngine& eng) const {
    return {eng.best_bid(), eng.best_ask()};
}

void MarketService::maybe_publish_depth(const std::string& symbol,
                                         MatchEngine& eng, TopOfBook before) {
    TopOfBook after = snapshot_tob(eng);
    if (after.bid == before.bid && after.ask == before.ask) return;

    market::DepthUpdate du;
    du.set_symbol(symbol);
    du.set_best_bid(after.bid);
    du.set_best_ask(after.ask);
    publish(cfg_.depth_topic, symbol, du);
}

void MarketService::publish(const std::string& topic, const std::string& key,
                             const google::protobuf::MessageLite& msg) {
    std::string payload;
    msg.SerializeToString(&payload);

    if (cfg_.test_publisher) {
        cfg_.test_publisher(topic, key, payload);
        return;
    }

    auto err = producer_->produce(
        topic,
        RdKafka::Topic::PARTITION_UA,
        RdKafka::Producer::RK_MSG_COPY,
        const_cast<char*>(payload.data()), payload.size(),
        key.data(), key.size(),
        0, nullptr, nullptr);

    if (err != RdKafka::ERR_NO_ERROR)
        std::cerr << "Produce failed: " << RdKafka::err2str(err) << '\n';
}

// ── Order handlers ───────────────────────────────────────────────────────────

void MarketService::handle_new_order(const std::string& symbol,
                                      const secwager::Order& o) {
    MatchEngine& eng = engine_for(symbol);
    auto before = snapshot_tob(eng);

    Order order{};
    std::memset(order.trader, 0, 8);
    std::memcpy(order.trader, o.trader().data(),
                std::min<size_t>(o.trader().size(), 8));
    order.price = static_cast<t_price>(o.price());
    order.size  = static_cast<t_size>(o.size());
    order.side  = (o.side() == secwager::SELL) ? 'S' : 'B';

    secwager::OrderStatus status;
    status.set_symbol(symbol);
    status.set_trader(o.trader());

    try {
        t_size total_filled = 0;
        fill_accumulator_ = &total_filled;
        t_orderid id = eng.limit(order);  // ExecCallback fires 0..N times here
        fill_accumulator_ = nullptr;

        t_size remaining = order.size - total_filled;
        status.set_order_id(id);
        status.set_status(secwager::ACCEPTED);
        status.set_remaining_size(remaining);
        order_remaining_[symbol][id] = remaining;
    } catch (const std::exception& ex) {
        fill_accumulator_ = nullptr;
        status.set_status(secwager::REJECTED);
        status.set_reject_reason(ex.what());
    }

    publish(cfg_.status_topic, symbol, status);
    maybe_publish_depth(symbol, eng, before);
}

void MarketService::handle_cancel(const std::string& symbol,
                                   const secwager::CancelRequest& cr) {
    secwager::OrderStatus status;
    status.set_symbol(symbol);
    status.set_order_id(cr.order_id());
    status.set_status(secwager::CANCELLED);

    auto it = engines_.find(symbol);
    if (it == engines_.end()) {
        // No engine for this symbol — order unknown; publish with remaining=0
        publish(cfg_.status_topic, symbol, status);
        return;
    }
    MatchEngine& eng = *it->second;
    auto before = snapshot_tob(eng);

    auto& sym_remaining = order_remaining_[symbol];
    auto rem_it = sym_remaining.find(cr.order_id());
    if (rem_it != sym_remaining.end()) {
        status.set_remaining_size(rem_it->second);
        sym_remaining.erase(rem_it);
    }

    eng.cancel(cr.order_id());

    publish(cfg_.status_topic, symbol, status);
    maybe_publish_depth(symbol, eng, before);
}
