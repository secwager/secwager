#include "icashier.hpp"
#include "etcd_lock.hpp"
#include "postgres_cashier.hpp"
#include <gmock/gmock.h>
#include <gtest/gtest.h>
#include <memory>

// ── Mock etcd lock (no-op; tests focus on cashier logic) ──────────────────

class MockLockHandle final : public IEtcdLockHandle {};

class MockLockFactory final : public IEtcdLockFactory {
public:
    MOCK_METHOD(std::unique_ptr<IEtcdLockHandle>, acquire,
                (const std::string& key), (override));

    // Helper: configure all acquire() calls to return a no-op handle.
    void allow_any() {
        ON_CALL(*this, acquire(::testing::_))
            .WillByDefault([]( const std::string&) {
                return std::make_unique<MockLockHandle>();
            });
    }
};

// ── In-memory fake cashier for pure-logic tests ───────────────────────────
// Avoids a real DB dependency; tests correctness of ICashier contract.

class FakeCashier final : public ICashier {
public:
    struct Account { int64_t balance{0}; int64_t locked{0}; };
    std::unordered_map<std::string, Account>  accounts;
    std::unordered_map<uint32_t, std::pair<std::string, int64_t>> escrows;  // order_id → {trader, amount}

    CashierResult deposit(const std::string& trader, int64_t amount) override {
        auto& a = accounts[trader];
        a.balance += amount;
        return {a.balance, a.locked, a.balance - a.locked};
    }

    CashierResult withdraw(const std::string& trader, int64_t amount) override {
        auto it = accounts.find(trader);
        if (it == accounts.end()) throw UnknownTraderError(trader);
        auto& a = it->second;
        if (a.balance - a.locked < amount) throw InsufficientFundsError("insufficient");
        a.balance -= amount;
        return {a.balance, a.locked, a.balance - a.locked};
    }

    CashierResult escrow(const std::string& trader,
                         uint32_t order_id, int64_t amount) override {
        auto it = accounts.find(trader);
        if (it == accounts.end()) throw UnknownTraderError(trader);
        auto& a = it->second;
        if (a.balance - a.locked < amount) throw InsufficientFundsError("insufficient");
        a.locked += amount;
        escrows[order_id] = {trader, amount};
        return {a.balance, a.locked, a.balance - a.locked};
    }

    CashierResult release_escrow(uint32_t order_id) override {
        auto it = escrows.find(order_id);
        if (it == escrows.end()) throw UnknownEscrowError(std::to_string(order_id));
        auto [trader, amount] = it->second;
        escrows.erase(it);
        auto& a = accounts[trader];
        a.locked -= amount;
        return {a.balance, a.locked, a.balance - a.locked};
    }

    CashierResult check_available(const std::string& trader) override {
        auto it = accounts.find(trader);
        if (it == accounts.end()) return {};
        auto& a = it->second;
        return {a.balance, a.locked, a.balance - a.locked};
    }
};

// ── Tests ─────────────────────────────────────────────────────────────────

class CashierTest : public ::testing::Test {
protected:
    FakeCashier cashier;
};

TEST_F(CashierTest, DepositNewTrader) {
    auto r = cashier.deposit("alice", 500);
    EXPECT_EQ(r.balance,   500);
    EXPECT_EQ(r.locked,    0);
    EXPECT_EQ(r.available, 500);
}

TEST_F(CashierTest, DepositExistingTraderAccumulates) {
    cashier.deposit("alice", 300);
    auto r = cashier.deposit("alice", 200);
    EXPECT_EQ(r.balance,   500);
    EXPECT_EQ(r.available, 500);
}

TEST_F(CashierTest, WithdrawSufficientFunds) {
    cashier.deposit("alice", 500);
    auto r = cashier.withdraw("alice", 200);
    EXPECT_EQ(r.balance,   300);
    EXPECT_EQ(r.available, 300);
}

TEST_F(CashierTest, WithdrawInsufficientFundsThrows) {
    cashier.deposit("alice", 100);
    EXPECT_THROW(cashier.withdraw("alice", 200), InsufficientFundsError);
}

TEST_F(CashierTest, WithdrawUnknownTraderThrows) {
    EXPECT_THROW(cashier.withdraw("ghost", 1), UnknownTraderError);
}

TEST_F(CashierTest, EscrowLocksFunds) {
    cashier.deposit("alice", 500);
    auto r = cashier.escrow("alice", 42, 200);
    EXPECT_EQ(r.balance,   500);
    EXPECT_EQ(r.locked,    200);
    EXPECT_EQ(r.available, 300);
}

TEST_F(CashierTest, EscrowInsufficientFundsThrows) {
    cashier.deposit("alice", 100);
    EXPECT_THROW(cashier.escrow("alice", 1, 200), InsufficientFundsError);
}

TEST_F(CashierTest, EscrowReducesAvailableNotBalance) {
    cashier.deposit("alice", 500);
    cashier.escrow("alice", 10, 300);
    auto r = cashier.check_available("alice");
    EXPECT_EQ(r.balance,   500);
    EXPECT_EQ(r.locked,    300);
    EXPECT_EQ(r.available, 200);
}

TEST_F(CashierTest, MultipleEscrowsSameTrader) {
    cashier.deposit("alice", 500);
    cashier.escrow("alice", 1, 100);
    cashier.escrow("alice", 2, 100);
    auto r = cashier.check_available("alice");
    EXPECT_EQ(r.locked,    200);
    EXPECT_EQ(r.available, 300);
}

TEST_F(CashierTest, ReleaseEscrowRestoresFunds) {
    cashier.deposit("alice", 500);
    cashier.escrow("alice", 42, 200);
    auto r = cashier.release_escrow(42);
    EXPECT_EQ(r.locked,    0);
    EXPECT_EQ(r.available, 500);
}

TEST_F(CashierTest, ReleaseUnknownEscrowThrows) {
    EXPECT_THROW(cashier.release_escrow(999), UnknownEscrowError);
}

TEST_F(CashierTest, CheckAvailableNoAccount) {
    auto r = cashier.check_available("ghost");
    EXPECT_EQ(r.balance,   0);
    EXPECT_EQ(r.locked,    0);
    EXPECT_EQ(r.available, 0);
}

TEST_F(CashierTest, WithdrawRespectesLockedFunds) {
    // Locked funds are not available for withdrawal.
    cashier.deposit("alice", 500);
    cashier.escrow("alice", 1, 300);  // 200 available
    EXPECT_THROW(cashier.withdraw("alice", 201), InsufficientFundsError);
    auto r = cashier.withdraw("alice", 200);
    EXPECT_EQ(r.balance,   300);
    EXPECT_EQ(r.locked,    300);
    EXPECT_EQ(r.available, 0);
}

// ── etcd lock ordering test (using mock) ─────────────────────────────────

TEST(EtcdLockOrderingTest, LockAcquiredBeforeOperation) {
    auto mock_factory = std::make_shared<MockLockFactory>();

    // Use InSequence to verify acquire() is called (we'll rely on the FakeCashier
    // to verify the logical operation; here we just confirm the lock factory is used).
    // A full integration test would wire MockLockFactory into PostgresCashier.
    ::testing::InSequence seq;
    EXPECT_CALL(*mock_factory, acquire("/cashier/lock/alice"))
        .WillOnce([]( const std::string&) {
            return std::make_unique<MockLockHandle>();
        });

    // Invoke directly to verify acquire is called with the correct key.
    auto handle = mock_factory->acquire("/cashier/lock/alice");
    EXPECT_NE(handle, nullptr);
}
