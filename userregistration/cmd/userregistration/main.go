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

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"google.golang.org/grpc"

	pb "github.com/secwager/secwager/proto/gen/userregistration"
	"github.com/secwager/secwager/userregistration/internal"
)

func main() {
	pgDSN      := mustEnv("USERREG_PG_DSN")
	listenAddr := envOr("USERREG_LISTEN_ADDR", "0.0.0.0:50052")
	kmsKeyARN  := mustEnv("USERREG_KMS_KEY_ARN")
	region     := envOr("USERREG_COGNITO_REGION", "us-east-1")
	poolSize   := parseInt(envOr("USERREG_PG_POOL_SIZE", "10"))
	btcNetwork := envOr("USERREG_BTC_NETWORK", "mainnet")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := runMigrations(pgDSN); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	pgCfg, err := pgxpool.ParseConfig(pgDSN)
	if err != nil {
		log.Fatalf("parse pg config: %v", err)
	}
	pgCfg.MaxConns = int32(poolSize)
	pool, err := pgxpool.NewWithConfig(ctx, pgCfg)
	if err != nil {
		log.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}

	kmsClient := kms.NewFromConfig(awsCfg)
	enc       := internal.NewKMSEncryptor(kmsClient, kmsKeyARN)
	svc       := internal.NewUserRegistrationService(pool, enc, kmsKeyARN, btcNetworkParams(btcNetwork))

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	pb.RegisterUserRegistrationServiceServer(srv, svc)

	log.Printf("userregistration listening on %s", listenAddr)
	go func() {
		<-ctx.Done()
		log.Println("shutting down")
		srv.GracefulStop()
	}()

	if err := srv.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
	log.Println("userregistration stopped")
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
	log.Println("migrations applied")
	return nil
}

func btcNetworkParams(network string) *chaincfg.Params {
	switch network {
	case "testnet3":
		return &chaincfg.TestNet3Params
	case "regtest":
		return &chaincfg.RegressionNetParams
	default:
		return &chaincfg.MainNetParams
	}
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
