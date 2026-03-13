#pragma once
#include <cstdint>
#include <cstring>
#include <functional>
#include <string>
#include <vector>

static constexpr uint32_t MAX_PRICE  = 65536;  // prices are uint16_t: 1–65535
static constexpr uint32_t MAX_ORDERS = 1 << 20; // 1M order slots in pool

using t_orderid = uint32_t;
using t_price   = uint16_t;
using t_size    = uint32_t;

struct Order {
    char    trader[8];
    t_price price;
    t_size  size;
    char    side;   // 'B' buy, 'S' sell
};

struct Execution {
    char    buyer[8];
    char    seller[8];
    t_price price;
    t_size  size;
};

using ExecCallback = std::function<void(const Execution&)>;

class MatchEngine {
public:
    explicit MatchEngine(ExecCallback cb);

    // Submit a limit order. Returns the assigned order ID.
    t_orderid limit(const Order& order);

    // Cancel a resting order by ID. No-op if already filled or unknown.
    void cancel(t_orderid id);

    // Clear all state (used internally by restore).
    void reset();

    // Serialize current book state to protobuf bytes for Kafka snapshot messages.
    // Only resting (unfilled / partially-filled) orders are captured.
    std::string serialize() const;

    // Restore book from protobuf bytes produced by serialize().
    // Calls reset() first. Preserves original order IDs so in-flight cancels resolve.
    void restore(const std::string& bytes);

    // Scan for the true best bid/ask, skipping empty and cancelled levels.
    t_price best_bid() const;
    t_price best_ask() const;

private:
    struct OrderEntry {
        char      trader[8];
        t_size    size;
        t_orderid next;     // index of next order at same price level (0 = none)
        t_price   price;
        char      side;
        bool      active;
    };

    struct PriceLevel {
        t_orderid head;  // front of FIFO queue (0 = empty)
        t_orderid tail;  // back  of FIFO queue (0 = empty)
    };

    t_orderid alloc(const Order& o);
    void      enqueue(PriceLevel& lvl, t_orderid id);
    void      skip_inactive(PriceLevel& lvl);

    ExecCallback on_exec_;

    // Heap-allocated: combined ~21 MB, far too large for the stack.
    std::vector<PriceLevel> bids_;  // size MAX_PRICE, buy side indexed by price
    std::vector<PriceLevel> asks_;  // size MAX_PRICE, sell side indexed by price
    std::vector<OrderEntry> pool_;  // size MAX_ORDERS; pool_[0] is the NONE sentinel

    uint32_t   pool_top_;  // bump allocator; valid IDs start at 1
    uint32_t   bid_max_;   // best bid price (0 = no bids)
    uint32_t   ask_min_;   // best ask price (MAX_PRICE = no asks)

    static constexpr t_orderid NONE = 0;
};
