#include "balance_service.h"
#include <unordered_map>

namespace secwager {
namespace cashier {

// Simple in-memory storage for now
static std::unordered_map<std::string, Balance> account_balances = {
    {"user123", {"user123", 1000.50, 250.00, 1250.50}},
    {"user456", {"user456", 500.75, 0.00, 500.75}},
    {"user789", {"user789", 2500.25, 100.00, 2600.25}}
};

BalanceService::BalanceService() {
}

std::optional<Balance> BalanceService::get_balance(const std::string& account_id) {
    auto it = account_balances.find(account_id);
    if (it != account_balances.end()) {
        return it->second;
    }
    return std::nullopt;
}


} // namespace cashier
} // namespace secwager