#pragma once
#include <string>
#include <memory>
#include <optional>

struct HttpResponse
{
    int status_code;
    std::string body;
};

class IHttpClient
{
public:
    virtual ~IHttpClient() = default;
    virtual std::optional<HttpResponse> Get(const std::string &path) = 0;
};
