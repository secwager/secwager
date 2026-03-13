#include "engine.hpp"
#include <gtest/gtest.h>
#include <vector>
#include <string>

// Helper to build an Order
static Order make_order(const char* trader, t_price price, t_size size, char side) {
    Order o{};
    std::strncpy(o.trader, trader, 8);
    o.price = price;
    o.size  = size;
    o.side  = side;
    return o;
}

class EngineTest : public ::testing::Test {
protected:
    std::vector<Execution> execs_;
    MatchEngine eng_{[this](const Execution& e) { execs_.push_back(e); }};

    void SetUp() override { execs_.clear(); }
};

// --- Basic matching ---

TEST_F(EngineTest, NoMatchWhenSpreadNotCrossed) {
    eng_.limit(make_order("buyer1  ", 100, 10, 'B'));
    eng_.limit(make_order("seller1 ", 101, 10, 'S'));
    EXPECT_TRUE(execs_.empty());
    EXPECT_EQ(eng_.best_bid(), 100);
    EXPECT_EQ(eng_.best_ask(), 101);
}

TEST_F(EngineTest, FullFillExactMatch) {
    eng_.limit(make_order("seller1 ", 100, 10, 'S'));
    eng_.limit(make_order("buyer1  ", 100, 10, 'B'));
    ASSERT_EQ(execs_.size(), 1u);
    EXPECT_EQ(execs_[0].price, 100);
    EXPECT_EQ(execs_[0].size,  10);
    EXPECT_EQ(std::string(execs_[0].buyer,  8), std::string("buyer1  "));
    EXPECT_EQ(std::string(execs_[0].seller, 8), std::string("seller1 "));
}

TEST_F(EngineTest, PartialFillBuyLargerThanResting) {
    eng_.limit(make_order("seller1 ", 100,  6, 'S'));
    eng_.limit(make_order("buyer1  ", 100, 10, 'B'));
    ASSERT_EQ(execs_.size(), 1u);
    EXPECT_EQ(execs_[0].size, 6);             // only 6 filled
    EXPECT_EQ(eng_.best_bid(), 100);           // 4 lots remain resting
    EXPECT_EQ(eng_.best_ask(), 0);             // ask side empty
}

TEST_F(EngineTest, PartialFillSellLargerThanResting) {
    eng_.limit(make_order("buyer1  ", 100,  4, 'B'));
    eng_.limit(make_order("seller1 ", 100, 10, 'S'));
    ASSERT_EQ(execs_.size(), 1u);
    EXPECT_EQ(execs_[0].size, 4);
    EXPECT_EQ(eng_.best_ask(), 100);           // 6 lots remain resting
    EXPECT_EQ(eng_.best_bid(), 0);             // bid side empty
}

TEST_F(EngineTest, AggressiveBuySweepsMultipleLevels) {
    eng_.limit(make_order("s1      ", 100, 5, 'S'));
    eng_.limit(make_order("s2      ", 101, 5, 'S'));
    eng_.limit(make_order("s3      ", 102, 5, 'S'));
    eng_.limit(make_order("buyer1  ", 102, 15, 'B'));  // sweeps all three levels
    ASSERT_EQ(execs_.size(), 3u);
    EXPECT_EQ(execs_[0].price, 100);
    EXPECT_EQ(execs_[1].price, 101);
    EXPECT_EQ(execs_[2].price, 102);
    EXPECT_EQ(eng_.best_ask(), 0);             // ask side cleared
}

// --- Price & time priority ---

TEST_F(EngineTest, PricePriorityBestAskFilledFirst) {
    eng_.limit(make_order("s_worst ", 102, 5, 'S'));
    eng_.limit(make_order("s_best  ", 100, 5, 'S'));
    eng_.limit(make_order("buyer1  ", 102, 5, 'B'));
    ASSERT_EQ(execs_.size(), 1u);
    EXPECT_EQ(execs_[0].price, 100);           // matched at best ask
    EXPECT_EQ(std::string(execs_[0].seller, 8), std::string("s_best  "));
}

TEST_F(EngineTest, TimePriorityFIFOWithinLevel) {
    eng_.limit(make_order("s_first ", 100, 5, 'S'));
    eng_.limit(make_order("s_second", 100, 5, 'S'));
    eng_.limit(make_order("buyer1  ", 100, 5, 'B'));
    ASSERT_EQ(execs_.size(), 1u);
    EXPECT_EQ(std::string(execs_[0].seller, 8), std::string("s_first "));
}

// --- Cancel ---

TEST_F(EngineTest, CancelledOrderNotMatched) {
    t_orderid id = eng_.limit(make_order("seller1 ", 100, 10, 'S'));
    eng_.cancel(id);
    eng_.limit(make_order("buyer1  ", 100, 10, 'B'));
    EXPECT_TRUE(execs_.empty());
}

TEST_F(EngineTest, CancelledOrderSkippedNextFills) {
    t_orderid id1 = eng_.limit(make_order("s_cancel", 100, 5, 'S'));
    eng_.limit(make_order("s_fill  ", 100, 5, 'S'));
    eng_.cancel(id1);
    eng_.limit(make_order("buyer1  ", 100, 5, 'B'));
    ASSERT_EQ(execs_.size(), 1u);
    EXPECT_EQ(std::string(execs_[0].seller, 8), std::string("s_fill  "));
}

TEST_F(EngineTest, CancelUnknownIdIsNoOp) {
    EXPECT_NO_THROW(eng_.cancel(999999));
}

// --- Reset ---

TEST_F(EngineTest, ResetClearsBook) {
    eng_.limit(make_order("seller1 ", 100, 10, 'S'));
    eng_.limit(make_order("buyer1  ", 99,  10, 'B'));
    eng_.reset();
    EXPECT_EQ(eng_.best_bid(), 0);
    EXPECT_EQ(eng_.best_ask(), 0);
    // After reset, orders should not match against stale state
    eng_.limit(make_order("seller2 ", 100, 10, 'S'));
    eng_.limit(make_order("buyer2  ", 100, 10, 'B'));
    ASSERT_EQ(execs_.size(), 1u);
    EXPECT_EQ(execs_[0].size, 10);
}

// --- Serialize / Restore ---

TEST_F(EngineTest, SerializeRestorePreservesRestingOrders) {
    eng_.limit(make_order("seller1 ", 100,  5, 'S'));
    eng_.limit(make_order("seller2 ", 101,  3, 'S'));
    eng_.limit(make_order("buyer1  ",  99, 10, 'B'));

    std::string snap = eng_.serialize();

    MatchEngine eng2([](const Execution&) {});
    eng2.restore(snap);

    EXPECT_EQ(eng2.best_bid(), 99);
    EXPECT_EQ(eng2.best_ask(), 100);

    // A crossing buy should match against restored asks
    std::vector<Execution> execs2;
    MatchEngine eng3([&execs2](const Execution& e) { execs2.push_back(e); });
    eng3.restore(snap);

    eng3.limit(make_order("buyer2  ", 101, 8, 'B'));
    ASSERT_EQ(execs2.size(), 2u);
    EXPECT_EQ(execs2[0].price, 100);
    EXPECT_EQ(execs2[0].size,   5);
    EXPECT_EQ(execs2[1].price, 101);
    EXPECT_EQ(execs2[1].size,   3);
}

TEST_F(EngineTest, SerializeRestorePreservesOriginalIds) {
    t_orderid id1 = eng_.limit(make_order("seller1 ", 100, 10, 'S'));
    // partial fill: 4 lots consumed, 6 remain
    eng_.limit(make_order("buyer1  ", 100,  4, 'B'));
    ASSERT_EQ(execs_.size(), 1u);

    std::string snap = eng_.serialize();

    std::vector<Execution> execs2;
    MatchEngine eng2([&execs2](const Execution& e) { execs2.push_back(e); });
    eng2.restore(snap);

    // Cancel the original order by its preserved ID — should prevent further fills
    eng2.cancel(id1);
    eng2.limit(make_order("buyer2  ", 100, 6, 'B'));
    EXPECT_TRUE(execs2.empty());
}

TEST_F(EngineTest, RestoreEmptySnapshotIsNoOp) {
    MatchEngine eng2([](const Execution&) {});
    std::string snap = eng_.serialize();  // empty book
    EXPECT_NO_THROW(eng2.restore(snap));
    EXPECT_EQ(eng2.best_bid(), 0);
    EXPECT_EQ(eng2.best_ask(), 0);
}

TEST_F(EngineTest, SerializeOmitsFullyFilledOrders) {
    eng_.limit(make_order("seller1 ", 100, 5, 'S'));
    eng_.limit(make_order("buyer1  ", 100, 5, 'B'));  // fully filled — both gone
    ASSERT_EQ(execs_.size(), 1u);

    std::string snap = eng_.serialize();

    MatchEngine eng2([](const Execution&) {});
    eng2.restore(snap);
    EXPECT_EQ(eng2.best_bid(), 0);
    EXPECT_EQ(eng2.best_ask(), 0);
}

TEST_F(EngineTest, SerializeRestorePartialFillCorrectRemainingSize) {
    // Place a 10-lot ask, partially fill 3, leaving 7 at rest
    eng_.limit(make_order("seller1 ", 100, 10, 'S'));
    eng_.limit(make_order("buyer1  ", 100,  3, 'B'));
    ASSERT_EQ(execs_.size(), 1u);
    EXPECT_EQ(execs_[0].size, 3);

    std::string snap = eng_.serialize();

    std::vector<Execution> execs2;
    MatchEngine eng2([&execs2](const Execution& e) { execs2.push_back(e); });
    eng2.restore(snap);

    // Only 7 lots should be at rest — a 7-lot buy should fully fill, not partially
    eng2.limit(make_order("buyer2  ", 100, 7, 'B'));
    ASSERT_EQ(execs2.size(), 1u);
    EXPECT_EQ(execs2[0].size, 7);        // exactly 7 filled
    EXPECT_EQ(eng2.best_ask(), 0);       // ask side now empty
}

TEST_F(EngineTest, SerializeCancelledOrdersExcluded) {
    t_orderid id1 = eng_.limit(make_order("seller1 ", 100, 5, 'S'));
    eng_.limit(make_order("seller2 ", 100, 5, 'S'));
    eng_.cancel(id1);   // cancel the first order

    std::string snap = eng_.serialize();

    std::vector<Execution> execs2;
    MatchEngine eng2([&execs2](const Execution& e) { execs2.push_back(e); });
    eng2.restore(snap);

    // Only seller2's 5 lots should be at rest
    eng2.limit(make_order("buyer1  ", 100, 10, 'B'));
    ASSERT_EQ(execs2.size(), 1u);
    EXPECT_EQ(execs2[0].size, 5);                                      // only 5, not 10
    EXPECT_EQ(std::string(execs2[0].seller, 8), std::string("seller2 "));
    EXPECT_EQ(eng2.best_ask(), 0);  // ask side empty (cancelled order never reappears)
}

TEST_F(EngineTest, SerializeRestoreMultipleOrdersSameLevelAllRestored) {
    eng_.limit(make_order("s1      ", 100, 2, 'S'));
    eng_.limit(make_order("s2      ", 100, 3, 'S'));
    eng_.limit(make_order("s3      ", 100, 4, 'S'));  // 9 lots total at price 100

    std::string snap = eng_.serialize();

    std::vector<Execution> execs2;
    MatchEngine eng2([&execs2](const Execution& e) { execs2.push_back(e); });
    eng2.restore(snap);

    // A 9-lot buy should sweep all three resting orders in FIFO order
    eng2.limit(make_order("buyer1  ", 100, 9, 'B'));
    ASSERT_EQ(execs2.size(), 3u);
    EXPECT_EQ(std::string(execs2[0].seller, 8), std::string("s1      "));
    EXPECT_EQ(execs2[0].size, 2);
    EXPECT_EQ(std::string(execs2[1].seller, 8), std::string("s2      "));
    EXPECT_EQ(execs2[1].size, 3);
    EXPECT_EQ(std::string(execs2[2].seller, 8), std::string("s3      "));
    EXPECT_EQ(execs2[2].size, 4);
    EXPECT_EQ(eng2.best_ask(), 0);
}

TEST_F(EngineTest, SerializeRestoreMultiplePriceLevelsBothSides) {
    // Bid side: levels at 98, 99
    eng_.limit(make_order("b1      ",  98, 3, 'B'));
    eng_.limit(make_order("b2      ",  99, 4, 'B'));
    // Ask side: levels at 101, 102
    eng_.limit(make_order("s1      ", 101, 5, 'S'));
    eng_.limit(make_order("s2      ", 102, 6, 'S'));

    std::string snap = eng_.serialize();

    std::vector<Execution> execs2;
    MatchEngine eng2([&execs2](const Execution& e) { execs2.push_back(e); });
    eng2.restore(snap);

    EXPECT_EQ(eng2.best_bid(), 99);
    EXPECT_EQ(eng2.best_ask(), 101);

    // Aggressive sell at 98 should sweep both bid levels (4 + 3 = 7 lots)
    eng2.limit(make_order("seller  ",  98, 7, 'S'));
    ASSERT_EQ(execs2.size(), 2u);
    EXPECT_EQ(execs2[0].price, 99);  // best bid filled first
    EXPECT_EQ(execs2[0].size,  4);
    EXPECT_EQ(execs2[1].price, 98);
    EXPECT_EQ(execs2[1].size,  3);
    EXPECT_EQ(eng2.best_bid(), 0);
    EXPECT_EQ(eng2.best_ask(), 101); // ask side untouched
}

TEST_F(EngineTest, SerializeRestoreNewOrdersGetFreshIds) {
    t_orderid id1 = eng_.limit(make_order("seller1 ", 100, 5, 'S'));
    t_orderid id2 = eng_.limit(make_order("seller2 ", 101, 5, 'S'));

    std::string snap = eng_.serialize();

    MatchEngine eng2([](const Execution&) {});
    eng2.restore(snap);

    // New orders must not reuse the IDs already in the snapshot
    t_orderid id3 = eng2.limit(make_order("seller3 ", 102, 5, 'S'));
    EXPECT_NE(id3, id1);
    EXPECT_NE(id3, id2);

    // Cancel original IDs — they should still resolve correctly
    eng2.cancel(id1);
    eng2.cancel(id2);

    // And cancel the new ID — no crash
    EXPECT_NO_THROW(eng2.cancel(id3));
}

TEST_F(EngineTest, SerializeRestorePreservesTimePriority) {
    eng_.limit(make_order("s_first ", 100, 5, 'S'));
    eng_.limit(make_order("s_second", 100, 5, 'S'));

    std::string snap = eng_.serialize();

    std::vector<Execution> execs2;
    MatchEngine eng2([&execs2](const Execution& e) { execs2.push_back(e); });
    eng2.restore(snap);

    eng2.limit(make_order("buyer1  ", 100, 5, 'B'));
    ASSERT_EQ(execs2.size(), 1u);
    EXPECT_EQ(std::string(execs2[0].seller, 8), std::string("s_first "));
}
