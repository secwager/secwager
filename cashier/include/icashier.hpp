#pragma once
#include <cstdint>
#include <stdexcept>
#include <string>

struct CashierResult {
    int64_t balance{0};    // total deposited (gross)
    int64_t locked{0};     // currently escrowed
    int64_t available{0};  // balance - locked
};

struct InsufficientFundsError : std::runtime_error {
    using std::runtime_error::runtime_error;
};
struct UnknownTraderError : std::runtime_error {
    using std::runtime_error::runtime_error;
};
struct UnknownEscrowError : std::runtime_error {
    using std::runtime_error::runtime_error;
};

class ICashier {
public:
    virtual ~ICashier() = default;

    // Increase balance. Creates account on first call. amount must be > 0.
    virtual CashierResult deposit(const std::string& trader, int64_t amount) = 0;

    // Decrease available balance. Throws InsufficientFundsError if available < amount.
    virtual CashierResult withdraw(const std::string& trader, int64_t amount) = 0;

    // Lock `amount` for order_id. Throws InsufficientFundsError if available < amount.
    // Caller is responsible for computing the correct lock amount.
    virtual CashierResult escrow(const std::string& trader,
                                 uint32_t           order_id,
                                 int64_t            amount) = 0;

    // Unlock escrowed funds when an order is cancelled.
    // Throws UnknownEscrowError if order_id not found.
    virtual CashierResult release_escrow(uint32_t order_id) = 0;

    // Returns current account snapshot. Returns zeroed CashierResult if trader doesn't exist.
    [[nodiscard]] virtual CashierResult check_available(const std::string& trader) = 0;
};
