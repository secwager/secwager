#include "market_service.hpp"
#include <csignal>
#include <cstdlib>
#include <iostream>
#include <stdexcept>
#include <string>

static MarketService* g_svc = nullptr;

static void on_signal(int) {
    if (g_svc) g_svc->stop();
}

int main() {
    ServiceConfig cfg;

    if (const char* v = std::getenv("KAFKA_BROKERS"))       cfg.brokers        = v;
    if (const char* v = std::getenv("KAFKA_GROUP_ID"))      cfg.group_id       = v;
    if (const char* v = std::getenv("KAFKA_IN_TOPIC"))      cfg.in_topic       = v;
    if (const char* v = std::getenv("KAFKA_EXEC_TOPIC"))    cfg.exec_topic     = v;
    if (const char* v = std::getenv("KAFKA_STATUS_TOPIC"))  cfg.status_topic   = v;
    if (const char* v = std::getenv("KAFKA_DEPTH_TOPIC"))   cfg.depth_topic    = v;
    if (const char* v = std::getenv("KAFKA_POLL_TIMEOUT_MS"))
        cfg.poll_timeout_ms = std::stoi(v);

    if (cfg.brokers.empty())  { std::cerr << "KAFKA_BROKERS not set\n"; return 1; }
    if (cfg.group_id.empty()) { std::cerr << "KAFKA_GROUP_ID not set\n"; return 1; }

    try {
        MarketService svc(cfg);
        g_svc = &svc;
        std::signal(SIGINT,  on_signal);
        std::signal(SIGTERM, on_signal);
        std::cout << "market service starting (brokers=" << cfg.brokers << ")\n";
        svc.run();
        std::cout << "market service stopped\n";
    } catch (const std::exception& ex) {
        std::cerr << "Fatal: " << ex.what() << '\n';
        return 1;
    }
    return 0;
}
