#pragma once

#include "balance_service.h"
#include <crow.h>
#include <memory>

namespace secwager {
namespace cashier {

class CashierController {
public:
    explicit CashierController(std::unique_ptr<BalanceService> service);
    ~CashierController() = default;

    void setup_routes(crow::SimpleApp& app);

private:
    std::unique_ptr<BalanceService> balance_service_;
    
    crow::response get_balance(const crow::request& req, const std::string& account_id);
};

} 
} 