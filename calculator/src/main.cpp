#include <iostream>
#include <crow.h>
#include "calculator.h"
#include "calculator_controller.h"

int main() {
    crow::SimpleApp app;
    secwager::calculator::Calculator calculator;
    secwager::calculator::CalculatorController controller(calculator);
    controller.set_up_routes(app);
    app.port(8080).multithreaded().run();
    return 0;
}