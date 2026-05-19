#include "postgres_cashier.hpp"
#include <pqxx/pqxx>
#include <stdexcept>

PostgresCashier::PostgresCashier(const std::string&               dsn,
                                 std::shared_ptr<IEtcdLockFactory> lock_factory,
                                 int                               pool_size)
    : pool_(dsn, pool_size)
    , lock_factory_(std::move(lock_factory))
{}

CashierResult PostgresCashier::fetch_result(pqxx::work& txn, const std::string& trader) {
    auto rows = txn.exec(
        "SELECT balance, locked FROM accounts WHERE trader_id = $1",
        pqxx::params{trader});
    if (rows.empty()) return {};
    return CashierResult{
        .balance   = rows[0][0].as<int64_t>(),
        .locked    = rows[0][1].as<int64_t>(),
        .available = rows[0][0].as<int64_t>() - rows[0][1].as<int64_t>(),
    };
}

CashierResult PostgresCashier::deposit(const std::string& trader, int64_t amount) {
    if (amount <= 0) throw std::invalid_argument("deposit amount must be > 0");

    auto lock = lock_factory_->acquire(lock_key(trader));
    auto conn = pool_.acquire();
    pqxx::work txn{*conn};
    try {
        txn.exec(
            R"(
            INSERT INTO accounts (trader_id, balance, locked, version)
            VALUES ($1, $2, 0, 0)
            ON CONFLICT (trader_id) DO UPDATE
                SET balance    = accounts.balance + $2,
                    version    = accounts.version + 1,
                    updated_at = clock_timestamp()
            )",
            pqxx::params{trader, amount});
        txn.commit();
    } catch (...) {
        txn.abort();
        throw;
    }
    // Re-read under the same lock (after commit, using a fresh txn)
    auto conn2 = pool_.acquire();
    pqxx::nontransaction rd{*conn2};
    auto row = rd.exec(
        "SELECT balance, locked FROM accounts WHERE trader_id = $1",
        pqxx::params{trader}).one_row();
    return CashierResult{
        .balance   = row[0].as<int64_t>(),
        .locked    = row[1].as<int64_t>(),
        .available = row[0].as<int64_t>() - row[1].as<int64_t>(),
    };
}

CashierResult PostgresCashier::withdraw(const std::string& trader, int64_t amount) {
    if (amount <= 0) throw std::invalid_argument("withdraw amount must be > 0");

    auto lock = lock_factory_->acquire(lock_key(trader));
    auto conn = pool_.acquire();
    pqxx::work txn{*conn};
    try {
        auto rows = txn.exec(
            "SELECT balance, locked, version FROM accounts WHERE trader_id = $1 FOR UPDATE",
            pqxx::params{trader});
        if (rows.empty()) throw UnknownTraderError("trader not found: " + trader);

        int64_t balance = rows[0][0].as<int64_t>();
        int64_t locked  = rows[0][1].as<int64_t>();
        int64_t version = rows[0][2].as<int64_t>();
        int64_t avail   = balance - locked;

        if (avail < amount)
            throw InsufficientFundsError("insufficient funds: available=" +
                                         std::to_string(avail) +
                                         " requested=" + std::to_string(amount));

        int64_t new_balance = balance - amount;
        auto updated = txn.exec(
            "UPDATE accounts SET balance = $1, version = version + 1, updated_at = clock_timestamp() "
            "WHERE trader_id = $2 AND version = $3",
            pqxx::params{new_balance, trader, version});
        if (updated.affected_rows() == 0)
            throw std::runtime_error("concurrent modification detected for trader: " + trader);

        txn.commit();
        return CashierResult{
            .balance   = new_balance,
            .locked    = locked,
            .available = new_balance - locked,
        };
    } catch (...) {
        txn.abort();
        throw;
    }
}

CashierResult PostgresCashier::escrow(const std::string& trader,
                                      uint32_t           order_id,
                                      int64_t            amount) {
    if (amount <= 0) throw std::invalid_argument("escrow amount must be > 0");

    auto lock = lock_factory_->acquire(lock_key(trader));
    auto conn = pool_.acquire();
    pqxx::work txn{*conn};
    try {
        auto rows = txn.exec(
            "SELECT balance, locked, version FROM accounts WHERE trader_id = $1 FOR UPDATE",
            pqxx::params{trader});
        if (rows.empty()) throw UnknownTraderError("trader not found: " + trader);

        int64_t balance = rows[0][0].as<int64_t>();
        int64_t locked  = rows[0][1].as<int64_t>();
        int64_t version = rows[0][2].as<int64_t>();
        int64_t avail   = balance - locked;

        if (avail < amount)
            throw InsufficientFundsError("insufficient funds: available=" +
                                         std::to_string(avail) +
                                         " requested=" + std::to_string(amount));

        int64_t new_locked = locked + amount;
        auto updated = txn.exec(
            "UPDATE accounts SET locked = $1, version = version + 1, updated_at = clock_timestamp() "
            "WHERE trader_id = $2 AND version = $3",
            pqxx::params{new_locked, trader, version});
        if (updated.affected_rows() == 0)
            throw std::runtime_error("concurrent modification detected for trader: " + trader);

        txn.exec(
            "INSERT INTO escrow_entries (order_id, trader_id, amount) VALUES ($1, $2, $3)",
            pqxx::params{order_id, trader, amount});

        txn.commit();
        return CashierResult{
            .balance   = balance,
            .locked    = new_locked,
            .available = balance - new_locked,
        };
    } catch (...) {
        txn.abort();
        throw;
    }
}

CashierResult PostgresCashier::release_escrow(uint32_t order_id) {
    // Unlocked read to discover the trader before acquiring the per-trader lock.
    // Safe: escrow rows are only deleted here or in settle(), both under the lock.
    std::string trader;
    int64_t     escrow_amount{0};
    {
        auto conn = pool_.acquire();
        pqxx::nontransaction rd{*conn};
        auto rows = rd.exec(
            "SELECT trader_id, amount FROM escrow_entries WHERE order_id = $1",
            pqxx::params{order_id});
        if (rows.empty()) throw UnknownEscrowError("escrow not found for order_id: " +
                                                    std::to_string(order_id));
        trader        = rows[0][0].as<std::string>();
        escrow_amount = rows[0][1].as<int64_t>();
    }

    auto lock = lock_factory_->acquire(lock_key(trader));
    auto conn = pool_.acquire();
    pqxx::work txn{*conn};
    try {
        auto deleted = txn.exec(
            "DELETE FROM escrow_entries WHERE order_id = $1 RETURNING trader_id, amount",
            pqxx::params{order_id});
        if (deleted.empty())
            throw UnknownEscrowError("escrow not found for order_id: " +
                                      std::to_string(order_id));

        auto rows = txn.exec(
            "SELECT balance, locked, version FROM accounts WHERE trader_id = $1 FOR UPDATE",
            pqxx::params{trader});
        if (rows.empty()) throw UnknownTraderError("trader not found: " + trader);

        int64_t balance    = rows[0][0].as<int64_t>();
        int64_t locked     = rows[0][1].as<int64_t>();
        int64_t version    = rows[0][2].as<int64_t>();
        int64_t new_locked = locked - escrow_amount;

        auto updated = txn.exec(
            "UPDATE accounts SET locked = $1, version = version + 1, updated_at = clock_timestamp() "
            "WHERE trader_id = $2 AND version = $3",
            pqxx::params{new_locked, trader, version});
        if (updated.affected_rows() == 0)
            throw std::runtime_error("concurrent modification detected for trader: " + trader);

        txn.commit();
        return CashierResult{
            .balance   = balance,
            .locked    = new_locked,
            .available = balance - new_locked,
        };
    } catch (...) {
        txn.abort();
        throw;
    }
}

CashierResult PostgresCashier::check_available(const std::string& trader) {
    // No etcd lock: advisory read. The enforcing check is inside escrow().
    auto conn = pool_.acquire();
    pqxx::nontransaction rd{*conn};
    auto rows = rd.exec(
        "SELECT balance, locked FROM accounts WHERE trader_id = $1",
        pqxx::params{trader});
    if (rows.empty()) return {};
    int64_t balance = rows[0][0].as<int64_t>();
    int64_t locked  = rows[0][1].as<int64_t>();
    return CashierResult{
        .balance   = balance,
        .locked    = locked,
        .available = balance - locked,
    };
}
