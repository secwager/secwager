#pragma once

#include <string>
#include <optional>

namespace secwager {
namespace cashier {

struct Balance {
    std::string account_id;
    double available_balance;
    double escrow_balance;
    double total_balance;
};

class BalanceService {
public:
    explicit BalanceService();
    ~BalanceService() = default;

    std::optional<Balance> get_balance(const std::string& account_id);

private:
    // For now, we'll use in-memory storage
    // Later this could connect to a database
};

} // namespace cashier
} // namespace secwager