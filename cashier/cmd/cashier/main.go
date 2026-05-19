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

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"google.golang.org/grpc"

	pb "github.com/secwager/secwager/cashier/gen/cashier"
	"github.com/secwager/secwager/cashier/internal"
)

func main() {
	pgDSN := mustEnv("CASHIER_PG_DSN")
	listenAddr := envOr("CASHIER_LISTEN_ADDR", "0.0.0.0:50051")
	poolSize := parseInt(envOr("CASHIER_PG_POOL_SIZE", "10"))

	runMigrations(pgDSN)

	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(pgDSN)
	if err != nil {
		log.Fatalf("parse pg config: %v", err)
	}
	cfg.MaxConns = int32(poolSize)
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		log.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	pb.RegisterCashierServiceServer(srv, internal.NewCashierService(internal.NewPostgresCashier(pool)))

	log.Printf("cashier listening on %s", listenAddr)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		log.Println("shutting down")
		srv.GracefulStop()
	}()

	if err := srv.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
	log.Println("cashier stopped")
}

func runMigrations(dsn string) {
	db, err := sql.Open("pgx", dsn)
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
