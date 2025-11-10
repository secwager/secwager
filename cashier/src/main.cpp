#include "cashier_controller.h"
#include "balance_service.h"
#include <crow.h>
#include <memory>
#include <iostream>

int main() {
    auto balance_service = std::make_unique<secwager::cashier::BalanceService>();
    auto controller = secwager::cashier::CashierController(std::move(balance_service));
    crow::SimpleApp app;
    controller.setup_routes(app);
    app.port(8080).multithreaded().run();
    return 0;
}