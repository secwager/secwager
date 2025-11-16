#include "calculator_controller.h"


namespace secwager::calculator {

CalculatorController::CalculatorController(Calculator& calculator)
    : calculator_(calculator) {}

    void CalculatorController::set_up_routes(crow::SimpleApp& app) {
    CROW_ROUTE(app, "/multiply/<int>/<int>").methods("GET"_method)
    ([this](int a, int b){
        int result = calculator_.multiply(a, b);
        crow::json::wvalue res;
        res["multiplicand"] = a;
        res["multiplier"] = b;
        res["result"] = result;
        return crow::response{res};
    });
}

}
