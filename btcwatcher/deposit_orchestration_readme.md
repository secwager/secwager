# BTC Deposit Orchestration

## Startup

Each node starts two independent goroutines:

```
goroutine 1: elector.Run(ctx)   — campaigns for etcd leadership, retries every 5s on failure
goroutine 2: watcher.Run(ctx)   — processes blocks and ticks; checks IsLeader() for cashier calls
```

Both start immediately. Standbys process the full chain from block one — they just don't call cashier.

---

## Block processing (all nodes)

Neutrino's rescan streams `BlockEvent` via two callbacks:

**`OnFilteredBlockConnected`** — for each relevant tx output matching a watched address:
```
txs.Insert(Deposit{txid, vout, userID, satoshis, height})
  → ON CONFLICT (txid, vout) DO UPDATE SET reorged_at = NULL WHERE reorged_at IS NOT NULL
```
Multiple nodes inserting the same deposit is safe. A reorged txn reappearing in the canonical chain is automatically "un-reorged" by the conflict handler.

Then, leader-only:
```
retryEscrows()        → ListNotEscrowed → DepositEscrowed(userID, satoshis, "txid:vout") → MarkEscrowed
checkConfirmations()  → ListReadyToConfirm(minDepth, height) → ConfirmDeposit(userID, satoshis, "txid:vout") → MarkConfirmed
```

**`OnFilteredBlockDisconnected`** (reorg) — all nodes:
```
DeleteFromHeight(height) → soft delete: UPDATE SET reorged_at = NOW() WHERE confirmed = FALSE
```
Confirmed deposits are never affected by reorgs.

---

## Cashier retry ticker (leader only, every 30s)

Runs independently of block events. Calls `BestBlock()` for current height:
```
retryEscrows()
cancelReorgedEscrows()
checkConfirmations(node.BestBlock())
```
This drains the cashier backlog after an etcd outage — the new leader doesn't wait for the next Bitcoin block.

---

## Idempotency layers

| Layer | Mechanism |
|---|---|
| DB insert | `ON CONFLICT (txid, vout)` — active rows are no-ops; reorged rows are un-reorged |
| Escrow call | `depositRef = txid:vout` → cashier `idempotency_keys` table |
| Confirm call | `"confirm:" + depositRef` → same table, separate namespace |
| Cancel call | `"cancel:" + depositRef` → same table, separate namespace |
| State machine | `ListNotEscrowed` / `ListReadyToConfirm` / `ListReorgedEscrowed` filters prevent re-firing |

---

## Failure scenarios

**Cashier temporarily down:**
Deposit stays `escrowed=FALSE`. `retryEscrows` skips it on error. Retried on the next block or the next 30s tick.

**etcd temporarily down:**
All nodes insert deposits normally. `IsLeader()` is false everywhere — cashier calls are skipped. Deposits accumulate in `btc_deposits` with `escrowed=FALSE`. When etcd recovers, one node wins the election within seconds. The cashier ticker fires within 30s and drains the entire backlog.

**Leader node crashes:**
Its etcd lease expires after TTL (default 60s). The next campaigning standby wins. It finds all pending deposits in the shared DB — its SPV has been running so it's already at chain tip. The cashier ticker fires within 30s.

**Standby node crashes:**
No impact on cashier flow. On restart it rejoins SPV sync and campaigns for leadership.

**Bitcoin reorg:**
`DeleteFromHeight` soft-deletes unconfirmed deposits at or above the disconnected height (`reorged_at = NOW()`). `ListNotEscrowed` and `ListReadyToConfirm` exclude soft-deleted rows, so no further cashier calls are made for them.

- If the txn **reappears** in the winning chain: the INSERT conflict handler clears `reorged_at`, the deposit resumes normal processing. Because `DepositEscrowed` is idempotent, re-calling it is safe.
- If the txn **does not reappear** after `ReorgCancelAge` (default 20 minutes — two Bitcoin block intervals): `cancelReorgedEscrows` calls `cashier.CancelDeposit` to reverse the escrow, then hard-deletes the row.

---

## User address polling

A separate ticker (default 60s, `PollInterval`) runs `pollNewUsers()` which diffs the live user list from the UserStore against the in-memory `addrIndex` and registers new BTC addresses with the neutrino rescan via `WatchAddresses`.

---

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `BTCWATCHER_PG_DSN` | required | Shared btc_deposits Postgres DSN |
| `BTCWATCHER_USERREG_PG_DSN` | required | User registration Postgres DSN |
| `BTCWATCHER_CASHIER_ADDR` | required | Cashier gRPC address |
| `ETCD_ENDPOINTS` | required | Comma-separated etcd endpoints |
| `BTCWATCHER_LEASE_TTL` | `60` | etcd leader lease TTL in seconds |
| `BTCWATCHER_CONFIRMATIONS` | `6` | BTC confirmations before funds available |
| `BTCWATCHER_NETWORK` | `mainnet` | `mainnet`, `testnet3`, or `regtest` |
| `BTCWATCHER_DATA_DIR` | `/data` | Neutrino header/filter storage |
| `BTCWATCHER_PEERS` | `` | Comma-separated BTC P2P peers |
| `BTCWATCHER_POLL_INTERVAL` | `60s` | User address poll interval |
| `BTCWATCHER_CASHIER_RETRY_INTERVAL` | `30s` | Cashier retry ticker interval |
| `BTCWATCHER_REORG_CANCEL_AGE` | `20m` | Min age before cancelling a reorged escrow |
