package main

import (
	"context"
	"database/sql"
	"log"
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
	"github.com/secwager/secwager/vendor_refdata/internal"
)

func main() {
	dsn := mustEnv("VENDOR_REFDATA_PG_DSN")
	apiKey := mustEnv("APISPORTS_KEY")
	rosterInterval := optDuration(envOr("VENDOR_REFDATA_ROSTER_INTERVAL", "24h"), 24*time.Hour)
	scheduleInterval := optDuration(envOr("VENDOR_REFDATA_SCHEDULE_INTERVAL", "24h"), 24*time.Hour)
	lineupInterval := optDuration(envOr("VENDOR_REFDATA_LINEUP_INTERVAL", "15m"), 15*time.Minute)
	lineupWindow := optDuration(envOr("VENDOR_REFDATA_LINEUP_WINDOW", "6h"), 6*time.Hour)
	migrationsPath := envOr("VENDOR_REFDATA_MIGRATIONS_PATH", "../../registry/db/migrations")

	year := time.Now().Year()
	if y := os.Getenv("VENDOR_REFDATA_SEASON"); y != "" {
		if n, err := strconv.Atoi(y); err == nil {
			year = n
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := runMigrations(dsn, migrationsPath); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	store := internal.NewStore(pool)
	mlbClient := internal.NewMLBClient()
	apClient := internal.NewAPISportsClient(apiKey)

	rosterPop := internal.NewRosterPopulator(mlbClient, apClient, store, year)
	schedulePop := internal.NewSchedulePopulator(mlbClient, apClient, store, year)
	lineupPop := internal.NewLineupPopulator(mlbClient, apClient, store, lineupWindow)

	go rosterPop.Run(ctx, rosterInterval)
	go schedulePop.Run(ctx, scheduleInterval)
	go lineupPop.Run(ctx, lineupInterval)

	<-ctx.Done()
	log.Printf("vendor_refdata: shutting down")
}

func runMigrations(dsn, migrationsPath string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsPath, "postgres", driver)
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

func optDuration(s string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

