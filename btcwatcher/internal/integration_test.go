//go:build integration

package internal

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"

	pb "github.com/secwager/secwager/proto/gen/cashier"
)

// integTestAddr is a fresh testnet3 P2WPKH address generated for this test.
// Send any amount of testnet3 BTC here while the test is running.
// WIF (for reference): cMjxxx28qg4i4QkVnx5A5eoZmJhxsDEkPX2EULtBf6RUv4NfZB4i
const (
	integTestAddr    = "tb1qt8ffv6rf7xtsrprkm8ul7ac8p0r47rahymuhc5"
	integTestUserID  = "integration-user-1"
	integTestDataDir = "/tmp/btcwatcher-integration-test"
)

func migrationsPath(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "../db/migrations")
}

func startPostgres(t *testing.T, ctx context.Context, user, pass, dbname string) string {
	t.Helper()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     user,
			"POSTGRES_PASSWORD": pass,
			"POSTGRES_DB":       dbname,
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}
	port, err := c.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("container port: %v", err)
	}
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port.Port(), user, pass, dbname)
}

func runMigrationsForTest(t *testing.T, dsn, migrationsDir string) {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open migration db: %v", err)
	}
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		t.Fatalf("migration driver: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsDir, "postgres", driver)
	if err != nil {
		t.Fatalf("migration init: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migration up: %v", err)
	}
}

// fakeCashierGRPCServer wraps fakeCashierClient as a gRPC server so the
// Watcher's grpcCashierClient can call it over a bufconn.
type fakeCashierGRPCServer struct {
	pb.UnimplementedCashierServiceServer
	client *fakeCashierClient
}

func (s *fakeCashierGRPCServer) DepositEscrowed(ctx context.Context, req *pb.DepositEscrowedRequest) (*pb.CashierResponse, error) {
	err := s.client.DepositEscrowed(ctx, req.UserId, req.Amount, req.DepositRef)
	if err != nil {
		return nil, err
	}
	return &pb.CashierResponse{}, nil
}

func (s *fakeCashierGRPCServer) ConfirmDeposit(ctx context.Context, req *pb.ConfirmDepositRequest) (*pb.CashierResponse, error) {
	err := s.client.ConfirmDeposit(ctx, req.UserId, req.Amount, req.DepositRef)
	if err != nil {
		return nil, err
	}
	return &pb.CashierResponse{}, nil
}

// TestIntegration_DepositEscrowAndConfirm runs a neutrino node on testnet3,
// waits for it to sync past a known historical transaction, and asserts that
// the Watcher correctly calls DepositEscrowed then ConfirmDeposit.
func TestIntegration_DepositEscrowAndConfirm(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	defer cancel()

	migDir := migrationsPath(t)
	dsn := startPostgres(t, ctx, "btcwatcher", "btcwatcher", "btcwatcher")
	runMigrationsForTest(t, dsn, migDir)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	// User store: one user mapped to the known testnet3 address.
	userStore := newFakeUserStore(UserRecord{
		UserID:  integTestUserID,
		BtcAddr: integTestAddr,
	})

	// Cashier: in-process fake exposed over bufconn.
	fakeCashier := newFakeCashierClient()
	lis := bufconn.Listen(1 << 20)
	grpcSrv := grpc.NewServer()
	pb.RegisterCashierServiceServer(grpcSrv, &fakeCashierGRPCServer{client: fakeCashier})
	go func() { _ = grpcSrv.Serve(lis) }()
	defer grpcSrv.Stop()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("bufconn dial: %v", err)
	}
	defer conn.Close()

	// Neutrino node on testnet3 via DNS peer discovery.
	dataDir := integTestDataDir
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("mkdir dataDir: %v", err)
	}
	params := &chaincfg.TestNet3Params

	node, err := NewNeutrinoSPVNode(dataDir, params, nil)
	if err != nil {
		t.Fatalf("neutrino node: %v", err)
	}
	defer node.Stop()

	w := NewWatcher(
		node,
		NewPostgresTxStore(pool),
		userStore,
		NewGRPCCashierClient(conn),
		Config{
			Confirmations: 1,
			PollInterval:  time.Minute,
			NetworkParams: params,
		},
	)

	watcherDone := make(chan error, 1)
	go func() { watcherDone <- w.Run(ctx) }()

	// Neutrino syncs to the current chain tip, then watches forward.
	// Send any amount of testnet3 BTC to the address below to trigger the test.
	t.Logf("========================================================")
	t.Logf("SEND TESTNET3 BTC TO: %s", integTestAddr)
	t.Logf("Waiting up to 10 minutes for a payment to arrive...")
	t.Logf("========================================================")

	waitFor(t, func() bool {
		fakeCashier.mu.Lock()
		defer fakeCashier.mu.Unlock()
		return len(fakeCashier.escrowedCalls) > 0
	}, 580*time.Second)

	fakeCashier.mu.Lock()
	ec := len(fakeCashier.escrowedCalls)
	fakeCashier.mu.Unlock()

	if ec == 0 {
		t.Fatal("DepositEscrowed was never called — no payment detected within timeout")
	}
	t.Logf("DepositEscrowed called %d time(s)", ec)

	// With Confirmations=1, ConfirmDeposit fires on the next block.
	t.Log("waiting for ConfirmDeposit (1 confirmation)...")
	waitFor(t, func() bool {
		fakeCashier.mu.Lock()
		defer fakeCashier.mu.Unlock()
		return len(fakeCashier.confirmCalls) > 0
	}, 30*time.Minute)

	fakeCashier.mu.Lock()
	cc := len(fakeCashier.confirmCalls)
	fakeCashier.mu.Unlock()

	if cc == 0 {
		t.Fatal("ConfirmDeposit was never called")
	}
	t.Logf("ConfirmDeposit called %d time(s)", cc)

	cancel()
	<-watcherDone
}
