package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/secwager/secwager/btcwatcher/internal"
)

func main() {
	pgDSN := mustEnv("BTCWATCHER_PG_DSN")
	userRegDSN := mustEnv("BTCWATCHER_USERREG_PG_DSN")
	cashierAddr := mustEnv("BTCWATCHER_CASHIER_ADDR")
	confirmations := parseInt(envOr("BTCWATCHER_CONFIRMATIONS", "6"))
	network := envOr("BTCWATCHER_NETWORK", "mainnet")
	dataDir := envOr("BTCWATCHER_DATA_DIR", "/data")
	peersRaw := envOr("BTCWATCHER_PEERS", "")
	pollInterval := parseDuration(envOr("BTCWATCHER_POLL_INTERVAL", "60s"))

	runMigrations(pgDSN)

	params := networkParams(network)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, pgDSN)
	if err != nil {
		log.Fatalf("btcwatcher db pool: %v", err)
	}
	defer pool.Close()

	userPool, err := pgxpool.New(ctx, userRegDSN)
	if err != nil {
		log.Fatalf("userregistration db pool: %v", err)
	}
	defer userPool.Close()

	conn, err := grpc.NewClient(cashierAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("grpc dial cashier: %v", err)
	}
	defer conn.Close()

	var peers []string
	if peersRaw != "" {
		peers = strings.Split(peersRaw, ",")
	}

	node, err := internal.NewNeutrinoSPVNode(dataDir, params, peers)
	if err != nil {
		log.Fatalf("new neutrino node: %v", err)
	}
	defer node.Stop()

	w := internal.NewWatcher(
		node,
		internal.NewPostgresTxStore(pool),
		internal.NewPostgresUserStore(userPool),
		internal.NewGRPCCashierClient(conn),
		internal.Config{
			Confirmations: confirmations,
			PollInterval:  pollInterval,
			NetworkParams: params,
		},
	)

	log.Printf("btcwatcher starting (network=%s, confirmations=%d)", network, confirmations)
	if err := w.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("watcher exited: %v", err)
	}
	log.Println("btcwatcher stopped")
}

func networkParams(network string) *chaincfg.Params {
	switch network {
	case "testnet3":
		return &chaincfg.TestNet3Params
	case "regtest":
		return &chaincfg.RegressionNetParams
	default:
		return &chaincfg.MainNetParams
	}
}

func runMigrations(pgDSN string) {
	db, err := sql.Open("pgx", pgDSN)
	if err != nil {
		log.Fatalf("open db for migrations: %v", err)
	}
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Fatalf("migration driver: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://db/migrations", "postgres", driver)
	if err != nil {
		log.Fatalf("migration init: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("migration up: %v", err)
	}
	log.Println("migrations applied")
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is required", key)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseInt(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		log.Fatalf("invalid int %q: %v", s, err)
	}
	return n
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Fatalf("invalid duration %q: %v", s, err)
	}
	return d
}
