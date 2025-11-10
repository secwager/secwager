#include "cashier_controller.h"

namespace secwager {
    namespace cashier {
        
        CashierController::CashierController(std::unique_ptr<BalanceService> service)
            : balance_service_(std::move(service)) {}

        void CashierController::setup_routes(crow::SimpleApp& app) {
            CROW_ROUTE(app, "/balance/<string>").methods("GET"_method)
            ([this](const crow::request& req, const std::string& account_id) {
                return this->get_balance(req, account_id);
            });
        }


        crow::response CashierController::get_balance(const crow::request& req, const std::string& account_id) {
            auto balance_opt = balance_service_->get_balance(account_id);
            if (balance_opt) {
                const Balance& balance = *balance_opt;
                crow::json::wvalue res;
                res["account_id"] = balance.account_id;
                res["available_balance"] = balance.available_balance;
                res["escrow_balance"] = balance.escrow_balance;
                res["total_balance"] = balance.total_balance;
                return crow::response{res};
            } else {
                return crow::response(404, "Account not found");
            }
        }

    }
}