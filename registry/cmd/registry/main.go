package main

import (
	"context"
	"database/sql"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	pb "github.com/secwager/secwager/proto/gen/registry"
	"github.com/secwager/secwager/registry/internal"
	"google.golang.org/grpc"
)

func main() {
	dsn := mustEnv("REGISTRY_PG_DSN")
	listenAddr := envOr("REGISTRY_LISTEN_ADDR", "0.0.0.0:50053")
	poolSize := optInt(envOr("REGISTRY_PG_POOL_SIZE", "10"))
	refreshInterval := optDuration(envOr("REGISTRY_REF_REFRESH_INTERVAL", "5m"))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Fatalf("parse dsn: %v", err)
	}
	cfg.MaxConns = int32(poolSize)
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	if err := runMigrations(dsn); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	cache := internal.NewRefCache(pool)
	cache.Start(ctx, refreshInterval)

	store := internal.NewPGStore(pool)
	svc := internal.NewRegistryService(cache, store)

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer( /* JWT interceptor slot */ )
	pb.RegisterRegistryServiceServer(srv, svc)

	log.Printf("registry listening on %s", listenAddr)
	go func() {
		<-ctx.Done()
		srv.GracefulStop()
	}()
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func runMigrations(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithDatabaseInstance("file://db/migrations", "postgres", driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func optInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func optDuration(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	if d <= 0 {
		d = 5 * time.Minute
	}
	return d
}
