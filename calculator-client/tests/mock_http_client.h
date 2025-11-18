#pragma once
#include "http_client.h"
#include <queue>

class MockHttpClient : public IHttpClient
{
public:
    // Queue responses to return on successive Get() calls
    void QueueResponse(std::optional<HttpResponse> response)
    {
        queued_responses.push(response);
    }

    std::optional<HttpResponse> Get(const std::string &path) override
    {
        last_path = path;

        if (queued_responses.empty())
        {
            // Default: successful response with result 42
            return HttpResponse{200, R"({"result": 42})"};
        }

        auto response = queued_responses.front();
        queued_responses.pop();
        return response;
    }

    // For verifying the path that was requested
    std::string GetLastPath() const
    {
        return last_path;
    }

private:
    std::queue<std::optional<HttpResponse>> queued_responses;
    std::string last_path;
};
