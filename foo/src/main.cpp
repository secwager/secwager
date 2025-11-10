#include <iostream>
#include "crow.h"
#include "calculator.h"

int main() {
    crow::SimpleApp app;
    secwager::foo::Calculator calculator;

    CROW_ROUTE(app, "/multiply/<int>/<int>")
    ([&calculator](int a, int b){
        int result = calculator.multiply(a, b);
        crow::json::wvalue
            return crow::response{std::to_string(result)};
    });

    app.port(18080).multithreaded().run();
    return 0;
}