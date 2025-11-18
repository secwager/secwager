#include "calculator_client.h"
#include <nlohmann/json.hpp>
#include <iostream>
#include <fmt/format.h>

CalculatorClient::CalculatorClient(std::unique_ptr<IHttpClient> http_client)
    : http_client{std::move(http_client)} {}

std::optional<int> CalculatorClient::multiply(int a, int b)
{
    std::string path = fmt::format("/multiply/{}/{}", a, b);
    auto response_opt = http_client->Get(path);

    if (!response_opt.has_value())
    {
        std::cerr << "Error: Network request failed" << std::endl;
        return std::nullopt;
    }

    auto response = response_opt.value();

    if (response.status_code != 200)
    {
        std::cerr << "Error: Received status code/body " << response.status_code << "/" << response.body << std::endl;
        return std::nullopt;
    }

    try
    {
        auto j = nlohmann::json::parse(response.body);
        return j["result"].get<int>();
    }
    catch (nlohmann::json::exception &e)
    {
        std::cerr << "Error: Failed to parse response body:" << response.body << " due to " << e.what() << std::endl;
        return std::nullopt;
    }
}