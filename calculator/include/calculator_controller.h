#pragma once
#include "crow.h"
#include "calculator.h"

namespace secwager::calculator {

class CalculatorController {
    public:
    explicit CalculatorController(Calculator& calculator);
    void set_up_routes(crow::SimpleApp& app);


    private:
    Calculator calculator_;

};

}