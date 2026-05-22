#pragma once
#include "icashier.hpp"
#include "etcd_lock.hpp"
#include <condition_variable>
#include <functional>
#include <memory>
#include <mutex>
#include <vector>
#include <pqxx/pqxx>

// Simple blocking connection pool. acquire() blocks until a connection is free.
struct PqxxConnectionPool {
    PqxxConnectionPool(const std::string& dsn, int size) {
        free_.reserve(size);
        for (int i = 0; i < size; ++i)
            free_.push_back(std::make_unique<pqxx::connection>(dsn));
    }

    // RAII handle — returns the connection to the pool on destruction.
    using Handle = std::unique_ptr<pqxx::connection,
                                   std::function<void(pqxx::connection*)>>;

    Handle acquire() {
        std::unique_lock lock{mu_};
        cv_.wait(lock, [&]{ return !free_.empty(); });
        auto conn = std::move(free_.back());
        free_.pop_back();
        pqxx::connection* raw = conn.release();
        return Handle{raw, [this](pqxx::connection* c) {
            std::lock_guard lk{mu_};
            free_.push_back(std::unique_ptr<pqxx::connection>(c));
            cv_.notify_one();
        }};
    }

private:
    std::mutex              mu_;
    std::condition_variable cv_;
    std::vector<std::unique_ptr<pqxx::connection>> free_;
};

class PostgresCashier final : public ICashier {
public:
    // dsn: libpq connection string, e.g. "host=... dbname=... user=... password=..."
    // pool_size: number of pooled connections
    PostgresCashier(const std::string&               dsn,
                    std::shared_ptr<IEtcdLockFactory> lock_factory,
                    int                               pool_size = 10);

    CashierResult deposit       (const std::string& trader, int64_t amount)           override;
    CashierResult withdraw      (const std::string& trader, int64_t amount)           override;
    CashierResult escrow        (const std::string& trader,
                                 uint32_t order_id, int64_t amount)                   override;
    CashierResult release_escrow(uint32_t order_id)                                   override;
    CashierResult check_available(const std::string& trader)                          override;

private:
    static std::string lock_key(const std::string& trader) {
        return "/cashier/lock/" + trader;
    }

    // Fetches current balance/locked for trader. Returns zeroed result if no row.
    CashierResult fetch_result(pqxx::work& txn, const std::string& trader);

    PqxxConnectionPool                pool_;
    std::shared_ptr<IEtcdLockFactory> lock_factory_;
};
