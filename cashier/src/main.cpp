#include "postgres_cashier.hpp"
#include "etcd_lock.hpp"
#include "cashier_service.hpp"
#include <grpcpp/grpcpp.h>
#include <csignal>
#include <cstdlib>
#include <iostream>
#include <memory>
#include <string>

// Forward declaration of factory defined in etcd_lock.cpp
std::unique_ptr<IEtcdLockFactory> make_etcd_lock_factory(const std::string& endpoints);

static std::function<void()> g_shutdown;

static void signal_handler(int) {
    if (g_shutdown) g_shutdown();
}

static std::string env_or(const char* name, std::string def) {
    if (const char* v = std::getenv(name)) return v;
    return def;
}

int main() {
    const std::string pg_dsn         = env_or("CASHIER_PG_DSN",         "");
    const std::string etcd_endpoints = env_or("CASHIER_ETCD_ENDPOINTS", "http://127.0.0.1:2379");
    const int         pool_size      = std::stoi(env_or("CASHIER_PG_POOL_SIZE", "10"));
    const std::string listen_addr    = env_or("CASHIER_LISTEN_ADDR",    "0.0.0.0:50051");

    if (pg_dsn.empty()) {
        std::cerr << "CASHIER_PG_DSN is required\n";
        return 1;
    }

    auto lock_factory = make_etcd_lock_factory(etcd_endpoints);
    auto pg_cashier   = std::make_shared<PostgresCashier>(pg_dsn,
                                                          std::move(lock_factory),
                                                          pool_size);
    CashierServiceImpl service(pg_cashier);

    grpc::ServerBuilder builder;
    builder.AddListeningPort(listen_addr, grpc::InsecureServerCredentials());
    builder.RegisterService(&service);
    auto server = builder.BuildAndStart();

    std::cout << "cashier listening on " << listen_addr << "\n";

    g_shutdown = [&] { server->Shutdown(); };
    std::signal(SIGINT,  signal_handler);
    std::signal(SIGTERM, signal_handler);

    server->Wait();
    std::cout << "cashier stopped\n";
    return 0;
}
