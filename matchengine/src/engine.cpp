#include "engine.hpp"
#include "book_snapshot.pb.h"
#include <algorithm>
#include <cstring>
#include <stdexcept>

MatchEngine::MatchEngine(ExecCallback cb)
    : on_exec_(std::move(cb))
    , bids_(MAX_PRICE)
    , asks_(MAX_PRICE)
    , pool_(MAX_ORDERS)
{
    reset();
}

void MatchEngine::reset() {
    std::memset(bids_.data(), 0, bids_.size() * sizeof(PriceLevel));
    std::memset(asks_.data(), 0, asks_.size() * sizeof(PriceLevel));
    std::memset(pool_.data(), 0, pool_.size() * sizeof(OrderEntry));
    pool_top_ = 1;      // 0 is the NONE sentinel
    bid_max_  = 0;      // no bids
    ask_min_  = MAX_PRICE;  // no asks
}

t_price MatchEngine::best_bid() const {
    for (int64_t p = static_cast<int64_t>(bid_max_); p > 0; --p) {
        t_orderid h = bids_[p].head;
        while (h != NONE && !pool_[h].active) h = pool_[h].next;
        if (h != NONE) return static_cast<t_price>(p);
    }
    return 0;
}

t_price MatchEngine::best_ask() const {
    for (uint32_t p = ask_min_; p < MAX_PRICE; ++p) {
        t_orderid h = asks_[p].head;
        while (h != NONE && !pool_[h].active) h = pool_[h].next;
        if (h != NONE) return static_cast<t_price>(p);
    }
    return 0;
}

t_orderid MatchEngine::alloc(const Order& o) {
    if (pool_top_ >= MAX_ORDERS)
        throw std::runtime_error("order pool exhausted");
    t_orderid id = pool_top_++;
    OrderEntry& e = pool_[id];
    std::memcpy(e.trader, o.trader, 8);
    e.size   = o.size;
    e.next   = NONE;
    e.price  = o.price;
    e.side   = o.side;
    e.active = true;
    return id;
}

void MatchEngine::enqueue(PriceLevel& lvl, t_orderid id) {
    pool_[id].next = NONE;
    if (lvl.tail != NONE)
        pool_[lvl.tail].next = id;
    else
        lvl.head = id;
    lvl.tail = id;
}

void MatchEngine::skip_inactive(PriceLevel& lvl) {
    while (lvl.head != NONE && !pool_[lvl.head].active)
        lvl.head = pool_[lvl.head].next;
    if (lvl.head == NONE)
        lvl.tail = NONE;
}

t_orderid MatchEngine::limit(const Order& order) {
    t_orderid id = alloc(order);

    if (order.side == 'B') {
        // Aggress the ask side
        while (pool_[id].size > 0 && ask_min_ <= static_cast<uint32_t>(order.price)) {
            PriceLevel& lvl = asks_[ask_min_];
            skip_inactive(lvl);
            if (lvl.head == NONE) {
                ++ask_min_;
                continue;
            }
            OrderEntry& resting = pool_[lvl.head];
            t_size fill = std::min(pool_[id].size, resting.size);

            Execution exec;
            std::memcpy(exec.buyer,  pool_[id].trader, 8);
            std::memcpy(exec.seller, resting.trader,   8);
            exec.price = resting.price;
            exec.size  = fill;
            on_exec_(exec);

            pool_[id].size -= fill;
            resting.size   -= fill;

            if (resting.size == 0) {
                resting.active = false;
                lvl.head = resting.next;
                if (lvl.head == NONE) {
                    lvl.tail = NONE;
                    ++ask_min_;
                }
            }
        }

        if (pool_[id].size > 0) {
            enqueue(bids_[order.price], id);
            if (static_cast<uint32_t>(order.price) > bid_max_)
                bid_max_ = order.price;
        } else {
            pool_[id].active = false;
        }

    } else {
        // Aggress the bid side
        while (pool_[id].size > 0 && bid_max_ >= static_cast<uint32_t>(order.price) && bid_max_ > 0) {
            PriceLevel& lvl = bids_[bid_max_];
            skip_inactive(lvl);
            if (lvl.head == NONE) {
                if (bid_max_ == 0) break;
                --bid_max_;
                continue;
            }
            OrderEntry& resting = pool_[lvl.head];
            t_size fill = std::min(pool_[id].size, resting.size);

            Execution exec;
            std::memcpy(exec.buyer,  resting.trader,   8);
            std::memcpy(exec.seller, pool_[id].trader, 8);
            exec.price = resting.price;
            exec.size  = fill;
            on_exec_(exec);

            pool_[id].size -= fill;
            resting.size   -= fill;

            if (resting.size == 0) {
                resting.active = false;
                lvl.head = resting.next;
                if (lvl.head == NONE) {
                    lvl.tail = NONE;
                    if (bid_max_ > 0) --bid_max_;
                }
            }
        }

        if (pool_[id].size > 0) {
            enqueue(asks_[order.price], id);
            if (static_cast<uint32_t>(order.price) < ask_min_)
                ask_min_ = order.price;
        } else {
            pool_[id].active = false;
        }
    }

    return id;
}

void MatchEngine::cancel(t_orderid id) {
    if (id == NONE || id >= pool_top_) return;
    pool_[id].active = false;
    pool_[id].size   = 0;
    // bid_max_ / ask_min_ are lazily corrected during the next limit() call
}

std::string MatchEngine::serialize() const {
    BookSnapshot snap;
    snap.set_best_bid(bid_max_);
    snap.set_best_ask(ask_min_);
    snap.set_pool_top(pool_top_);

    // Bids: high to low (best price first)
    for (uint32_t p = bid_max_; p > 0; --p) {
        t_orderid cur = bids_[p].head;
        while (cur != NONE) {
            const OrderEntry& e = pool_[cur];
            if (e.active) {
                auto* o = snap.add_orders();
                o->set_id(cur);
                o->set_trader(std::string(e.trader, strnlen(e.trader, 8)));
                o->set_price(e.price);
                o->set_size(e.size);
                o->set_side("B");
            }
            cur = e.next;
        }
    }

    // Asks: low to high (best price first)
    for (uint32_t p = ask_min_; p < MAX_PRICE; ++p) {
        t_orderid cur = asks_[p].head;
        while (cur != NONE) {
            const OrderEntry& e = pool_[cur];
            if (e.active) {
                auto* o = snap.add_orders();
                o->set_id(cur);
                o->set_trader(std::string(e.trader, strnlen(e.trader, 8)));
                o->set_price(e.price);
                o->set_size(e.size);
                o->set_side("S");
            }
            cur = e.next;
        }
    }

    return snap.SerializeAsString();
}

void MatchEngine::restore(const std::string& bytes) {
    reset();

    BookSnapshot snap;
    if (!snap.ParseFromString(bytes))
        throw std::runtime_error("failed to parse BookSnapshot");

    uint32_t max_id = 0;

    for (const auto& o : snap.orders()) {
        t_orderid id = static_cast<t_orderid>(o.id());
        if (id == NONE || id >= MAX_ORDERS)
            throw std::runtime_error("invalid order ID in snapshot");

        OrderEntry& e = pool_[id];
        const std::string& tr = o.trader();
        std::memset(e.trader, 0, 8);
        std::memcpy(e.trader, tr.data(), std::min<size_t>(tr.size(), 8));
        e.size   = static_cast<t_size>(o.size());
        e.next   = NONE;
        e.price  = static_cast<t_price>(o.price());
        e.side   = o.side().empty() ? 'B' : o.side()[0];
        e.active = true;

        if (e.side == 'B')
            enqueue(bids_[e.price], id);
        else
            enqueue(asks_[e.price], id);

        if (id > max_id) max_id = id;
    }

    // Restore trackers from snapshot (avoids O(MAX_PRICE) rescan)
    bid_max_  = snap.best_bid();
    ask_min_  = snap.best_ask() == 0 ? MAX_PRICE : snap.best_ask();
    pool_top_ = snap.pool_top() > max_id + 1 ? snap.pool_top() : max_id + 1;
    if (pool_top_ < 1) pool_top_ = 1;
}
