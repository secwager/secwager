#pragma once
#include <string>
#include <optional>
#include <memory>
#include "http_client.h"

class CalculatorClient
{

public:
    explicit CalculatorClient(std::unique_ptr<IHttpClient> http_client);
    std::optional<int> multiply(int a, int b);

private:
    std::unique_ptr<IHttpClient> http_client;
};
