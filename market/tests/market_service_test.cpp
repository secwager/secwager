#include <gtest/gtest.h>
#include "market_service.hpp"
#include "market.pb.h"
#include <vector>

struct Published {
    std::string topic;
    std::string key;
    std::string payload;
};

static ServiceConfig make_test_config(std::vector<Published>& out) {
    ServiceConfig cfg;
    cfg.test_publisher = [&out](const std::string& t,
                                const std::string& k,
                                const std::string& p) {
        out.push_back({t, k, p});
    };
    return cfg;
}

// ── helpers ──────────────────────────────────────────────────────────────────

static secwager::MarketCommand make_order(const std::string& trader,
                                          uint32_t price, uint32_t size,
                                          secwager::Side side) {
    secwager::Order o;
    o.set_trader(trader);
    o.set_price(price);
    o.set_size(size);
    o.set_side(side);
    secwager::MarketCommand cmd;
    *cmd.mutable_new_order() = o;
    return cmd;
}

static secwager::MarketCommand make_cancel(uint32_t order_id) {
    secwager::CancelRequest cr;
    cr.set_order_id(order_id);
    secwager::MarketCommand cmd;
    *cmd.mutable_cancel() = cr;
    return cmd;
}

// ── tests ─────────────────────────────────────────────────────────────────────

TEST(MarketServiceTest, NonCrossingOrderAccepted) {
    std::vector<Published> published;
    MarketService svc(make_test_config(published));

    svc.process("AAPL", make_order("alice", 100, 10, secwager::BUY));

    // Expect: one OrderStatus{ACCEPTED} and one DepthUpdate (bid 0→100)
    bool found_status = false;
    bool found_depth  = false;
    for (const auto& p : published) {
        if (p.topic == "order_status") {
            secwager::OrderStatus st;
            ASSERT_TRUE(st.ParseFromString(p.payload));
            EXPECT_EQ(st.status(),         secwager::ACCEPTED);
            EXPECT_EQ(st.remaining_size(), 10u);
            EXPECT_GT(st.order_id(),       0u);
            EXPECT_EQ(p.key,               "AAPL");
            found_status = true;
        }
        if (p.topic == "depth_updates") {
            market::DepthUpdate du;
            ASSERT_TRUE(du.ParseFromString(p.payload));
            EXPECT_EQ(du.best_bid(), 100u);
            EXPECT_EQ(du.best_ask(), 0u);
            found_depth = true;
        }
    }
    EXPECT_TRUE(found_status);
    EXPECT_TRUE(found_depth);
    // No executions expected
    for (const auto& p : published)
        EXPECT_NE(p.topic, "executions");
}

TEST(MarketServiceTest, CrossingOrderProducesExecution) {
    std::vector<Published> published;
    MarketService svc(make_test_config(published));

    // Rest a sell order at 100
    svc.process("AAPL", make_order("bob", 100, 10, secwager::SELL));
    published.clear();

    // Aggressive buy at 100 for size 5 — should fill completely
    svc.process("AAPL", make_order("alice", 100, 5, secwager::BUY));

    bool found_exec   = false;
    bool found_status = false;
    for (const auto& p : published) {
        if (p.topic == "executions") {
            secwager::Execution ex;
            ASSERT_TRUE(ex.ParseFromString(p.payload));
            EXPECT_EQ(ex.price(),  100u);
            EXPECT_EQ(ex.size(),   5u);
            EXPECT_EQ(ex.buyer(),  "alice");
            EXPECT_EQ(ex.seller(), "bob");
            found_exec = true;
        }
        if (p.topic == "order_status") {
            secwager::OrderStatus st;
            ASSERT_TRUE(st.ParseFromString(p.payload));
            EXPECT_EQ(st.status(),         secwager::ACCEPTED);
            EXPECT_EQ(st.remaining_size(), 0u);  // fully filled
            found_status = true;
        }
    }
    EXPECT_TRUE(found_exec);
    EXPECT_TRUE(found_status);
}

TEST(MarketServiceTest, FullyCrossingOrderNoDepthUpdate) {
    std::vector<Published> published;
    MarketService svc(make_test_config(published));

    // Rest a sell at 100
    svc.process("AAPL", make_order("seller", 100, 5, secwager::SELL));
    published.clear();

    // Buy for exactly the same size — consumes the resting order completely,
    // ask goes back to 0.  Depth changes, so a DepthUpdate IS expected.
    svc.process("AAPL", make_order("buyer", 100, 5, secwager::BUY));

    bool found_depth = false;
    for (const auto& p : published)
        if (p.topic == "depth_updates") found_depth = true;
    EXPECT_TRUE(found_depth);
}

TEST(MarketServiceTest, CancelRestingOrder) {
    std::vector<Published> published;
    MarketService svc(make_test_config(published));

    svc.process("AAPL", make_order("alice", 100, 10, secwager::BUY));

    // Extract the assigned order_id
    uint32_t order_id = 0;
    for (const auto& p : published) {
        if (p.topic == "order_status") {
            secwager::OrderStatus st;
            st.ParseFromString(p.payload);
            order_id = st.order_id();
        }
    }
    ASSERT_GT(order_id, 0u);
    published.clear();

    svc.process("AAPL", make_cancel(order_id));

    bool found_cancelled = false;
    bool found_depth     = false;
    for (const auto& p : published) {
        if (p.topic == "order_status") {
            secwager::OrderStatus st;
            ASSERT_TRUE(st.ParseFromString(p.payload));
            EXPECT_EQ(st.status(),         secwager::CANCELLED);
            EXPECT_EQ(st.order_id(),       order_id);
            EXPECT_EQ(st.remaining_size(), 10u);
            found_cancelled = true;
        }
        if (p.topic == "depth_updates") found_depth = true;
    }
    EXPECT_TRUE(found_cancelled);
    EXPECT_TRUE(found_depth);  // bid drops from 100 to 0
}

TEST(MarketServiceTest, CancelUnknownSymbolNoEngine) {
    std::vector<Published> published;
    MarketService svc(make_test_config(published));

    svc.process("ZZZZ", make_cancel(42));

    ASSERT_EQ(published.size(), 1u);
    EXPECT_EQ(published[0].topic, "order_status");
    secwager::OrderStatus st;
    ASSERT_TRUE(st.ParseFromString(published[0].payload));
    EXPECT_EQ(st.status(), secwager::CANCELLED);
}

TEST(MarketServiceTest, MultiSymbolEnginesAreIsolated) {
    std::vector<Published> published;
    MarketService svc(make_test_config(published));

    svc.process("AAPL", make_order("alice", 100, 5, secwager::BUY));
    svc.process("MSFT", make_order("bob",   200, 3, secwager::SELL));
    // Place a crossing AAPL sell — should match alice, not touch MSFT book
    svc.process("AAPL", make_order("charlie", 100, 5, secwager::SELL));

    int exec_count = 0;
    for (const auto& p : published) {
        if (p.topic == "executions") {
            secwager::Execution ex;
            ex.ParseFromString(p.payload);
            EXPECT_EQ(ex.symbol(), "AAPL");  // no MSFT crosses
            ++exec_count;
        }
    }
    EXPECT_EQ(exec_count, 1);
}
