# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**secwager** is a binary options trading platform built as a set of independent C++20 services. Each module is a self-contained CMake project with its own `vcpkg.json` and `CMakePresets.json`. There is no top-level CMake; build each module independently.

## Build System

Each module uses CMake + vcpkg + Ninja. `VCPKG_ROOT` must be set in the environment.

```bash
# Build any module (same pattern for matchengine/, market/, cashier/)
cd <module>
cmake --preset default
cmake --build build

# Run all unit tests
cd build && ctest --output-on-failure

# Run a single test binary directly
./build/engine_test                   # matchengine
./build/market_test                   # market
./build/cashier_test                  # cashier (unit, no external deps)

# Run integration tests (requires Docker)
cd cashier
docker compose run --rm integration-test
```

## Module Structure

```
proto/              — shared protobuf messages (no gRPC; consumed by all modules)
matchengine/        — price-time-priority match engine (static library + binary)
market/             — Kafka consumer/producer wrapping the match engine (gRPC-free)
cashier/            — gRPC service for trader balance management (Postgres + etcd)
```

### proto/
Single CMake target `secwager_proto` generating C++ from:
- `secwager/common.proto` — core trading messages: `Order`, `CancelRequest`, `MarketCommand`, `Execution`, `OrderStatus`
- `secwager/book_snapshot.proto` — serialized order book for engine restore
- `cashier/cashier.proto` — cashier gRPC service definition (message types only; stubs are generated inside the cashier module because gRPC is a cashier-only dependency)

Each module adds `proto/` via `add_subdirectory` with a guard (`if(NOT TARGET secwager_proto)`) to prevent double-registration when market pulls in matchengine, which also references proto.

### matchengine/
Price-time-priority FIFO limit order book. Fixed-size array-backed pool (`MAX_ORDERS = 1M`), array-indexed price levels (`MAX_PRICE = 65536`), covering uint16_t prices for binary options (1–65535). No Kafka, no gRPC — pure matching logic. Supports `serialize()`/`restore()` via protobuf bytes for snapshot/recovery.

### market/
Stateless Kafka consumer loop (`incoming_orders` topic) wrapping one `MatchEngine` per symbol. Publishes to three topics: `executions`, `order_status`, `depth_updates`. The `ServiceConfig::test_publisher` hook bypasses Kafka in unit tests — use `MarketService::process()` to inject commands directly. Dependencies: librdkafka, protobuf.

### cashier/
Standalone gRPC service (port 50051). `ICashier` interface (`cashier/include/icashier.hpp`) is the only public API — `PostgresCashier` is the sole concrete implementation. Per-account distributed locking via etcd (`/cashier/lock/<trader_id>`). Connection pooling via `pqxx::connection_pool` (built into libpqxx 7+). Schema managed by Liquibase YAML in `cashier/db/`; migrations run automatically in docker-compose. Integration tests spin up Postgres + etcd via the Docker CLI (not mocks).

## Key Design Decisions

- **ICashier is product-agnostic.** `escrow(trader, order_id, amount)` takes a caller-computed lock amount. Binary options liability math (max 100 - price) belongs in the order gateway, not the cashier.
- **Synchronous gRPC handlers.** Each handler blocks on libpqxx directly. Concurrency comes from gRPC's thread pool + `pqxx::connection_pool`. No Boost.Asio or async gRPC.
- **No ISqlRepository abstraction.** Different DB backends mean different `ICashier` implementations, not a swappable repo layer.
- **Proto lives in `proto/`, not inside modules.** Exception: `market/proto/market.proto` is market-specific (`DepthUpdate`) and generated locally.
- **cashier gRPC stubs are generated in cashier/, not proto/.** gRPC is a cashier-only dependency; `proto/CMakeLists.txt` uses only `protobuf_generate` (no grpc plugin).

## Kafka Topics

| Topic | Producer | Consumer |
|---|---|---|
| `incoming_orders` | order_gateway (external) | market |
| `executions` | market | risk, reporting |
| `order_status` | market | order_gateway |
| `depth_updates` | market | market data clients |

## cashier Environment Variables

| Variable | Example |
|---|---|
| `CASHIER_PG_DSN` | `host=localhost dbname=cashier user=cashier password=cashier` |
| `CASHIER_ETCD_ENDPOINTS` | `http://localhost:2379` |
| `CASHIER_LISTEN_ADDR` | `0.0.0.0:50051` |
