#include "cpr_http_client.h"

CprHttpClient::CprHttpClient(std::string base_url)
    : base_url{std::move(base_url)} {}

std::optional<HttpResponse> CprHttpClient::Get(const std::string &path)
{
    std::string full_url = base_url + path;
    auto r = cpr::Get(cpr::Url{full_url});

    if (r.error)
    {
        return std::nullopt;
    }

    // Successful network request (may still be HTTP error like 404, 500)
    return HttpResponse{
        .status_code = static_cast<int>(r.status_code),
        .body = r.text};
}
