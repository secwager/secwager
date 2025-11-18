#pragma once
#include "http_client.h"
#include <cpr/cpr.h>

class CprHttpClient : public IHttpClient
{
public:
    explicit CprHttpClient(std::string base_url);
    std::optional<HttpResponse> Get(const std::string &path) override;

private:
    std::string base_url;
};
