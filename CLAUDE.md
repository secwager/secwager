# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**secwager** is a binary options trading platform built as a set of independent Go modules sharing a single `go.work` workspace. Each module is self-contained with its own `go.mod`.

## Build System

```bash
# Run unit tests for all modules
go test ./cashier/... ./matchengine/... ./market/...

# Run unit tests for a specific module
go test github.com/secwager/secwager/matchengine/...
go test github.com/secwager/secwager/market/internal/service
go test github.com/secwager/secwager/cashier/internal/...

# Run integration tests (requires Docker)
TESTCONTAINERS_RYUK_DISABLED=true go test -tags integration \
  github.com/secwager/secwager/cashier/internal/... -timeout 120s
TESTCONTAINERS_RYUK_DISABLED=true go test -tags integration \
  github.com/secwager/secwager/market/internal/service -timeout 180s

# Build binaries
go build github.com/secwager/secwager/cashier/cmd/cashier
go build github.com/secwager/secwager/market/cmd/market

# Regenerate all proto stubs (run from repo root)
PATH=$PATH:$HOME/go/bin protoc \
  --go_out=proto/gen --go_opt=paths=source_relative \
  -I proto \
  secwager/common.proto secwager/book_snapshot.proto

PATH=$PATH:$HOME/go/bin protoc \
  --go_out=proto/gen --go_opt=paths=source_relative \
  --go-grpc_out=proto/gen --go-grpc_opt=paths=source_relative \
  -I proto cashier/cashier.proto

PATH=$PATH:$HOME/go/bin protoc \
  --go_out=proto/gen --go_opt=paths=source_relative \
  -I proto \
  market/market.proto market/market_snapshot.proto
```

## Module Structure

```
go.work             — workspace root; lists all modules
proto/              — all .proto sources + generated Go stubs (proto/gen/)
matchengine/        — pure Go matching library; no I/O, no external deps
market/             — Kafka consumer binary wrapping matchengine
cashier/            — gRPC service for trader balance management (Postgres)
```

### proto/
Single Go module (`github.com/secwager/secwager/proto`). All `.proto` sources live here; generated stubs go into `proto/gen/`. Other modules reference the stubs via `replace` directives pointing to `../proto`.

- `secwager/common.proto` — `Order`, `CancelRequest`, `MarketCommand`, `ExecutionReport`, `ExecType`, `RejectReason`
- `secwager/book_snapshot.proto` — serialized order book for snapshot/restore
- `cashier/cashier.proto` — cashier gRPC service + message types
- `market/market.proto` — `DepthUpdate`
- `market/market_snapshot.proto` — `MarketSnapshot` (offset + per-symbol books, written atomically)

### matchengine/
Pure Go limit order book. Fixed-size pool (`MaxOrders = 1M`), array-indexed price levels (`MaxPrice = 65536`), price-time FIFO. No Kafka, no proto, no goroutines — single-threaded per book. Callback-driven via `Callbacks` struct; nil fields are no-ops.

Key types: `OrderBook`, `Order`, `OrderEvent`, `BookState`, `Callbacks`, `ExecType` (`ExecNew`, `ExecTraded`, `ExecCancelled`, `ExecRejected`), `RejectReason`.

### market/
Kafka consumer binary. One `OrderBook` per symbol, keyed by the Kafka message key. Manual partition assignment via `MARKET_CARDINAL` — no consumer group, no rebalancing. Offset self-managed on disk inside `MarketSnapshot`. On startup: restore from snapshot, replay to HWM with nil callbacks, then go live. Snapshot written atomically (tmp + rename) on shutdown and every `FlushEvery` messages.

Publishes `ExecutionReport` to `order_executions` and `DepthUpdate` to `depth_updates`.

Unit tests use `Config.Publisher` hook (no Kafka). Integration tests use Redpanda via testcontainers.

### cashier/
Standalone gRPC service (port 50051). `Cashier` interface in `cashier/internal/cashier.go`; `PostgresCashier` is the production implementation. Operations: `Deposit`, `Withdraw`, `Escrow`, `ReleaseEscrow`, `CheckAvailable`. All mutating ops are idempotent via `idempotency_keys` table. Schema managed by `golang-migrate` SQL files in `cashier/db/migrations/`.

Unit tests use `FakeCashier`. Integration tests use testcontainers (Postgres).

## Key Design Decisions

- **Cashier is product-agnostic.** `Escrow(userID, orderID, amount)` takes a caller-computed amount. Binary options liability math belongs in the order gateway.
- **All protos in one place.** `proto/` is the single module for all `.proto` sources and generated Go stubs. No per-module gen directories.
- **matchengine has zero external dependencies.** It is a pure library — no Kafka, no proto, no I/O. The market layer owns all that.
- **Manual Kafka partition assignment.** `MARKET_CARDINAL` pins each market instance to a specific partition. No consumer group means no rebalancing on a stateful in-memory book.
- **Callbacks suppressed during replay.** `s.live = false` until replay is complete; all `OnOrder`/`OnBBOChange` closures are no-ops during startup replay to prevent duplicate output messages.
- **Atomic snapshots.** `MarketSnapshot` proto written to `.tmp` then renamed — POSIX rename is atomic, ensuring offset and book state are always consistent.
- **Idempotency via presence, not cached responses.** Cashier `idempotency_keys` is a presence-only table. On a cache hit, the current account state is returned with `is_replay=true`; no frozen response stored.

## Kafka Topics

| Topic | Producer | Consumer |
|---|---|---|
| `incoming_orders` | order_gateway (external) | market |
| `order_executions` | market | risk, reporting, order_gateway |
| `depth_updates` | market | market data clients |

## Environment Variables

### cashier
| Variable | Example |
|---|---|
| `CASHIER_PG_DSN` | `host=localhost dbname=cashier user=cashier password=cashier` |
| `CASHIER_LISTEN_ADDR` | `0.0.0.0:50051` |

### market
| Variable | Default | Description |
|---|---|---|
| `KAFKA_BROKERS` | (required) | Comma-separated broker list |
| `MARKET_CARDINAL` | (required) | Partition index this instance owns (0-based) |
| `KAFKA_IN_TOPIC` | `incoming_orders` | Consume topic |
| `KAFKA_EXEC_TOPIC` | `order_executions` | Publish ExecutionReports |
| `KAFKA_DEPTH_TOPIC` | `depth_updates` | Publish DepthUpdates |
| `MARKET_DATA_DIR` | `/data` | Snapshot directory |
| `MARKET_FLUSH_EVERY` | `1000` | Snapshot interval (messages) |
