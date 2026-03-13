#pragma once
#include <atomic>
#include <functional>
#include <memory>
#include <string>
#include <unordered_map>
#include <google/protobuf/message_lite.h>
#include <librdkafka/rdkafkacpp.h>
#include "engine.hpp"
#include "secwager/common.pb.h"

struct ServiceConfig {
    std::string brokers;
    std::string group_id;
    std::string in_topic     = "incoming_orders";
    std::string exec_topic   = "executions";
    std::string status_topic = "order_status";
    std::string depth_topic  = "depth_updates";
    int poll_timeout_ms      = 100;

    // When set, publish() calls this instead of Kafka (used in unit tests).
    std::function<void(const std::string& topic,
                       const std::string& key,
                       const std::string& payload)> test_publisher;
};

class MarketService {
public:
    explicit MarketService(const ServiceConfig& cfg);

    void run();   // blocking event loop
    void stop();  // sets atomic flag; safe from signal handler

    // Feed a command directly without Kafka (used in unit tests).
    void process(const std::string& symbol, const secwager::MarketCommand& cmd);

private:
    MatchEngine& engine_for(const std::string& symbol);
    void handle_new_order(const std::string& symbol, const secwager::Order&);
    void handle_cancel   (const std::string& symbol, const secwager::CancelRequest&);
    void publish(const std::string& topic, const std::string& key,
                 const google::protobuf::MessageLite& msg);

    struct TopOfBook { t_price bid; t_price ask; };
    TopOfBook snapshot_tob(MatchEngine&) const;
    void      maybe_publish_depth(const std::string& symbol,
                                  MatchEngine& eng, TopOfBook before);

    ServiceConfig cfg_;
    std::atomic<bool> running_{false};
    std::unique_ptr<RdKafka::KafkaConsumer> consumer_;
    std::unique_ptr<RdKafka::Producer>      producer_;
    std::unordered_map<std::string, std::unique_ptr<MatchEngine>> engines_;
    // Best-effort remaining size per order_id per symbol (for cancel status)
    std::unordered_map<std::string,
        std::unordered_map<t_orderid, t_size>> order_remaining_;

    // Non-owning pointer into the current handle_new_order stack frame;
    // null outside of a limit() call.
    t_size* fill_accumulator_ = nullptr;
};
