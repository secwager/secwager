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

## UI (ui/)

React 18 + TypeScript + Vite SPA. Requires Node 20+.

### First-time setup

```bash
# 1. Install JS dependencies
cd ui && npm install

# 2. Generate TypeScript proto stubs (requires protoc on PATH)
cd ui && npm run gen-proto

# 3. Copy env template and fill in values
cp ui/.env.example ui/.env.local
# Edit ui/.env.local — set VITE_COGNITO_USER_POOL_ID and VITE_COGNITO_CLIENT_ID
```

### Local dev with Docker Compose (recommended)

All backend services (Postgres, cashier, registry, userregistration, Envoy) start together.
The AWS-backed env vars must be real — Compose cannot fake Cognito or KMS.

```bash
# Supply required AWS env vars, then bring everything up
export USERREG_COGNITO_POOL_ID=us-east-1_Abc123
export USERREG_KMS_KEY_ARN=arn:aws:kms:us-east-1:123456789012:key/...

docker-compose up --build   # first run builds images; subsequent runs skip --build

# In a second terminal — start the Vite dev server
cd ui && npm run dev         # http://localhost:5173
```

Tear down (keeps Postgres volume):
```bash
docker-compose down
```

Tear down and wipe the database volume:
```bash
docker-compose down -v
```

### Local dev — manual start order

If you prefer running backends directly with `go run` (faster iteration, no Docker rebuilds):

**1. Postgres** — create databases once:
```bash
psql -U postgres -c "CREATE DATABASE cashier;"
psql -U postgres -c "CREATE DATABASE registry;"
psql -U postgres -c "CREATE DATABASE userregistration;"
```

**2. Cashier** (port 50051)
```bash
CASHIER_PG_DSN="host=localhost dbname=cashier user=postgres password=postgres sslmode=disable" \
go run github.com/secwager/secwager/cashier/cmd/cashier
```

**3. Registry** (port 50053)
```bash
REGISTRY_PG_DSN="host=localhost dbname=registry user=postgres password=postgres sslmode=disable" \
go run github.com/secwager/secwager/registry/cmd/registry
```

**4. Userregistration** (port 50052)
```bash
USERREG_PG_DSN="host=localhost dbname=userregistration user=postgres password=postgres sslmode=disable" \
USERREG_COGNITO_POOL_ID="us-east-1_XXXXXXXXX" \
USERREG_KMS_KEY_ARN="arn:aws:kms:us-east-1:123456789012:key/..." \
go run github.com/secwager/secwager/userregistration/cmd/userregistration
```

**5. Envoy** — when running backends on the host, Docker can't reach them via service name,
so point the cluster hostnames at the Docker bridge IP:

```bash
# Linux
docker run --rm -p 8080:8080 \
  --add-host=registry:host-gateway \
  --add-host=cashier:host-gateway \
  --add-host=userregistration:host-gateway \
  -v $PWD/ui/envoy/envoy.yaml:/etc/envoy/envoy.yaml \
  envoyproxy/envoy:v1.30-latest

# macOS — host-gateway not supported; use host.docker.internal
docker run --rm -p 8080:8080 \
  --add-host=registry:host.docker.internal \
  --add-host=cashier:host.docker.internal \
  --add-host=userregistration:host.docker.internal \
  -v $PWD/ui/envoy/envoy.yaml:/etc/envoy/envoy.yaml \
  envoyproxy/envoy:v1.30-latest
```

**6. Vite dev server** (http://localhost:5173)
```bash
cd ui && npm run dev
```

### Other UI commands

```bash
# Production build (output in ui/dist/)
cd ui && npm run build

# Run unit tests (watch mode)
cd ui && npm test

# Run tests once (CI)
cd ui && npm run test:run

# Coverage report
cd ui && npm run test:coverage
```

### Envoy ports

| Port | Purpose |
|------|---------|
| 8080 | grpc-web listener (Vite dev server talks here) |
| 9901 | Envoy admin UI (stats, health) |

### Backend ports

| Service | Port | Env var |
|---------|------|---------|
| cashier | 50051 | `CASHIER_LISTEN_ADDR` |
| userregistration | 50052 | `USERREG_LISTEN_ADDR` |
| registry | 50053 | `REGISTRY_LISTEN_ADDR` |

### Environment variables — UI

| Variable | Description |
|----------|-------------|
| `VITE_ENVOY_HOST` | Envoy base URL seen by the browser (default `http://localhost:8080`) |
| `VITE_COGNITO_USER_POOL_ID` | Cognito user pool ID (e.g. `us-east-1_Abc123`) |
| `VITE_COGNITO_CLIENT_ID` | Cognito app client ID |

Copy `ui/.env.example` to `ui/.env.local` and fill in Cognito and Envoy values before running.

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
