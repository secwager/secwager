//go:build integration

package internal

import (
	"context"
	"database/sql"
	"errors"
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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/secwager/secwager/cashier/gen/cashier"
)

var (
	testPool *pgxpool.Pool
	testDSN  string
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "cashier",
			"POSTGRES_PASSWORD": "cashier",
			"POSTGRES_DB":       "cashier",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}
	pgContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "start postgres container: %v\n", err)
		os.Exit(1)
	}
	defer pgContainer.Terminate(ctx)

	host, _ := pgContainer.Host(ctx)
	port, _ := pgContainer.MappedPort(ctx, "5432")
	testDSN = fmt.Sprintf("host=%s port=%s user=cashier password=cashier dbname=cashier sslmode=disable", host, port.Port())

	runTestMigrations(testDSN)

	pool, err := pgxpool.New(ctx, testDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create pool: %v\n", err)
		os.Exit(1)
	}
	testPool = pool
	defer pool.Close()

	os.Exit(m.Run())
}

func truncateTables(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`TRUNCATE TABLE idempotency_keys, escrow_entries, accounts RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
}

func dbSnapshot(t *testing.T, userID string) AccountSnapshot {
	t.Helper()
	var snap AccountSnapshot
	err := testPool.QueryRow(context.Background(),
		`SELECT gross_balance, escrowed FROM accounts WHERE user_id = $1`, userID).
		Scan(&snap.GrossBalance, &snap.Escrowed)
	if errors.Is(err, pgxErrNoRows()) {
		return AccountSnapshot{}
	}
	if err != nil {
		t.Fatalf("dbSnapshot: %v", err)
	}
	return snap
}

func dbEscrowExists(t *testing.T, orderID uint32) bool {
	t.Helper()
	var count int
	testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM escrow_entries WHERE order_id = $1`, orderID).Scan(&count)
	return count > 0
}

func pgxErrNoRows() error {
	// Import-free way to get the sentinel without importing pgx in test helpers.
	row := testPool.QueryRow(context.Background(), `SELECT 1 WHERE false`)
	var dummy int
	return row.Scan(&dummy)
}

// ── DB-backed cashier tests ────────────────────────────────────────────────

func newCashier() *PostgresCashier {
	return NewPostgresCashier(testPool)
}

func TestDB_Deposit_NewUser(t *testing.T) {
	truncateTables(t)
	c := newCashier()
	ctx := context.Background()

	snap, err := c.Deposit(ctx, "alice", "k1", 100)
	if err != nil {
		t.Fatal(err)
	}
	if snap.GrossBalance != 100 || snap.Escrowed != 0 {
		t.Fatalf("response: %+v", snap)
	}
	db := dbSnapshot(t, "alice")
	if db != snap {
		t.Fatalf("DB mismatch: response=%+v db=%+v", snap, db)
	}
}

func TestDB_Deposit_Idempotent(t *testing.T) {
	truncateTables(t)
	c := newCashier()
	ctx := context.Background()

	snap1, _ := c.Deposit(ctx, "alice", "k1", 100)
	snap2, _ := c.Deposit(ctx, "alice", "k1", 100)
	if snap1.GrossBalance != snap2.GrossBalance || snap1.Escrowed != snap2.Escrowed {
		t.Fatalf("idempotency broken: %+v vs %+v", snap1, snap2)
	}
	if snap1.IsReplay {
		t.Fatal("first call should not be a replay")
	}
	if !snap2.IsReplay {
		t.Fatal("second call should be a replay")
	}
	db := dbSnapshot(t, "alice")
	if db.GrossBalance != 100 {
		t.Fatalf("double-applied: db=%+v", db)
	}
}

func TestDB_Withdraw_SufficientFunds(t *testing.T) {
	truncateTables(t)
	c := newCashier()
	ctx := context.Background()

	c.Deposit(ctx, "alice", "d1", 200)
	snap, err := c.Withdraw(ctx, "alice", "w1", 80)
	if err != nil {
		t.Fatal(err)
	}
	if snap.GrossBalance != 120 {
		t.Fatalf("response: %+v", snap)
	}
	db := dbSnapshot(t, "alice")
	if db != snap {
		t.Fatalf("DB mismatch: response=%+v db=%+v", snap, db)
	}
}

func TestDB_Withdraw_InsufficientFunds(t *testing.T) {
	truncateTables(t)
	c := newCashier()
	ctx := context.Background()

	c.Deposit(ctx, "alice", "d1", 50)
	_, err := c.Withdraw(ctx, "alice", "w1", 100)
	var ife *InsufficientFundsError
	if !errors.As(err, &ife) {
		t.Fatalf("expected InsufficientFundsError, got %v", err)
	}
}

func TestDB_Withdraw_UnknownUser(t *testing.T) {
	truncateTables(t)
	c := newCashier()
	_, err := c.Withdraw(context.Background(), "ghost", "w1", 10)
	var uue *UnknownUserError
	if !errors.As(err, &uue) {
		t.Fatalf("expected UnknownUserError, got %v", err)
	}
}

func TestDB_Escrow_LocksFunds(t *testing.T) {
	truncateTables(t)
	c := newCashier()
	ctx := context.Background()

	c.Deposit(ctx, "alice", "d1", 100)
	snap, err := c.Escrow(ctx, "alice", 1, 60)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Escrowed != 60 {
		t.Fatalf("response: %+v", snap)
	}
	db := dbSnapshot(t, "alice")
	if db != snap {
		t.Fatalf("DB mismatch: response=%+v db=%+v", snap, db)
	}
	if !dbEscrowExists(t, 1) {
		t.Fatal("escrow_entries row missing")
	}
}

func TestDB_Escrow_BlocksWithdrawal(t *testing.T) {
	truncateTables(t)
	c := newCashier()
	ctx := context.Background()

	c.Deposit(ctx, "alice", "d1", 100)
	c.Escrow(ctx, "alice", 1, 80)
	_, err := c.Withdraw(ctx, "alice", "w1", 30)
	var ife *InsufficientFundsError
	if !errors.As(err, &ife) {
		t.Fatalf("expected InsufficientFundsError, got %v", err)
	}
}

func TestDB_Escrow_Idempotent(t *testing.T) {
	truncateTables(t)
	c := newCashier()
	ctx := context.Background()

	c.Deposit(ctx, "alice", "d1", 100)
	snap1, _ := c.Escrow(ctx, "alice", 42, 50)
	snap2, _ := c.Escrow(ctx, "alice", 42, 50)
	if snap1 != snap2 {
		t.Fatalf("idempotency broken: %+v vs %+v", snap1, snap2)
	}
	db := dbSnapshot(t, "alice")
	if db.Escrowed != 50 {
		t.Fatalf("double-applied escrow: db=%+v", db)
	}
}

func TestDB_ReleaseEscrow_RestoresFunds(t *testing.T) {
	truncateTables(t)
	c := newCashier()
	ctx := context.Background()

	c.Deposit(ctx, "alice", "d1", 100)
	c.Escrow(ctx, "alice", 1, 60)
	snap, err := c.ReleaseEscrow(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Escrowed != 0 {
		t.Fatalf("response: %+v", snap)
	}
	db := dbSnapshot(t, "alice")
	if db != snap {
		t.Fatalf("DB mismatch: response=%+v db=%+v", snap, db)
	}
	if dbEscrowExists(t, 1) {
		t.Fatal("escrow_entries row should be deleted")
	}
}

func TestDB_ReleaseEscrow_UnknownEscrow(t *testing.T) {
	truncateTables(t)
	c := newCashier()
	_, err := c.ReleaseEscrow(context.Background(), 999)
	var uee *UnknownEscrowError
	if !errors.As(err, &uee) {
		t.Fatalf("expected UnknownEscrowError, got %v", err)
	}
}

func TestDB_CheckAvailable_UnknownUserReturnsZero(t *testing.T) {
	truncateTables(t)
	c := newCashier()
	snap, err := c.CheckAvailable(context.Background(), "ghost")
	if err != nil || snap != (AccountSnapshot{}) {
		t.Fatalf("expected zero snapshot, got snap=%+v err=%v", snap, err)
	}
}

func TestDB_MultipleEscrows(t *testing.T) {
	truncateTables(t)
	c := newCashier()
	ctx := context.Background()

	c.Deposit(ctx, "alice", "d1", 100)
	c.Escrow(ctx, "alice", 1, 30)
	c.Escrow(ctx, "alice", 2, 30)
	snap, err := c.Escrow(ctx, "alice", 3, 30)
	if err != nil {
		t.Fatal(err)
	}
	db := dbSnapshot(t, "alice")
	if db.Escrowed != 90 || db != snap {
		t.Fatalf("response=%+v db=%+v", snap, db)
	}
}


func TestDB_IdempotencyCacheHit_ReleasesRowLock(t *testing.T) {
	truncateTables(t)
	c := newCashier()
	ctx := context.Background()

	c.Deposit(ctx, "alice", "k1", 100)

	// Second call with same key hits the cache and returns current state.
	snap, err := c.Deposit(ctx, "alice", "k1", 999)
	if err != nil {
		t.Fatal(err)
	}
	if snap.GrossBalance != 100 {
		t.Fatalf("expected gross_balance=100, got %+v", snap)
	}
	if !snap.IsReplay {
		t.Fatal("expected IsReplay=true on cache hit")
	}

	// If the deferred rollback didn't release the idempotency_keys row lock,
	// this Withdraw (which opens its own transaction and acquires FOR UPDATE) would hang.
	tctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err = c.Withdraw(tctx, "alice", "w1", 10)
	if err != nil {
		t.Fatalf("lock not released after idempotency cache hit: %v", err)
	}
	db := dbSnapshot(t, "alice")
	if db.GrossBalance != 90 {
		t.Fatalf("expected gross_balance=90 after withdraw, got %+v", db)
	}
}

// ── gRPC handler tests ────────────────────────────────────────────────────

func newGRPCClient(t *testing.T) (pb.CashierServiceClient, func()) {
	t.Helper()
	const bufSize = 1 << 20
	lis := bufconn.Listen(bufSize)

	srv := grpc.NewServer()
	cashier := NewPostgresCashier(testPool)
	pb.RegisterCashierServiceServer(srv, NewCashierService(cashier))
	go srv.Serve(lis)

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	return pb.NewCashierServiceClient(conn), func() {
		conn.Close()
		srv.GracefulStop()
	}
}

func TestGRPC_Deposit_ResponseAndDB(t *testing.T) {
	truncateTables(t)
	client, cleanup := newGRPCClient(t)
	defer cleanup()
	ctx := context.Background()

	resp, err := client.Deposit(ctx, &pb.DepositRequest{UserId: "bob", Amount: 200, IdempotencyKey: "k1"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GrossBalance != 200 || resp.Escrowed != 0 {
		t.Fatalf("response: %+v", resp)
	}
	db := dbSnapshot(t, "bob")
	if db.GrossBalance != resp.GrossBalance || db.Escrowed != resp.Escrowed {
		t.Fatalf("DB mismatch: resp=%+v db=%+v", resp, db)
	}
}

func TestGRPC_Escrow_ResponseAndDB(t *testing.T) {
	truncateTables(t)
	client, cleanup := newGRPCClient(t)
	defer cleanup()
	ctx := context.Background()

	client.Deposit(ctx, &pb.DepositRequest{UserId: "bob", Amount: 200, IdempotencyKey: "k1"})
	resp, err := client.Escrow(ctx, &pb.EscrowRequest{UserId: "bob", OrderId: 7, Amount: 90})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Escrowed != 90 {
		t.Fatalf("response: %+v", resp)
	}
	db := dbSnapshot(t, "bob")
	if db.GrossBalance != resp.GrossBalance || db.Escrowed != resp.Escrowed {
		t.Fatalf("DB mismatch: resp=%+v db=%+v", resp, db)
	}
	if !dbEscrowExists(t, 7) {
		t.Fatal("escrow_entries row missing")
	}
}

func TestGRPC_ReleaseEscrow_ResponseAndDB(t *testing.T) {
	truncateTables(t)
	client, cleanup := newGRPCClient(t)
	defer cleanup()
	ctx := context.Background()

	client.Deposit(ctx, &pb.DepositRequest{UserId: "bob", Amount: 200, IdempotencyKey: "k1"})
	client.Escrow(ctx, &pb.EscrowRequest{UserId: "bob", OrderId: 7, Amount: 90})
	resp, err := client.ReleaseEscrow(ctx, &pb.ReleaseEscrowRequest{OrderId: 7})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Escrowed != 0 {
		t.Fatalf("response: %+v", resp)
	}
	db := dbSnapshot(t, "bob")
	if db.GrossBalance != resp.GrossBalance || db.Escrowed != resp.Escrowed {
		t.Fatalf("DB mismatch: resp=%+v db=%+v", resp, db)
	}
	if dbEscrowExists(t, 7) {
		t.Fatal("escrow_entries row should be deleted")
	}
}

func TestGRPC_InsufficientFunds_StatusCode(t *testing.T) {
	truncateTables(t)
	client, cleanup := newGRPCClient(t)
	defer cleanup()
	ctx := context.Background()

	client.Deposit(ctx, &pb.DepositRequest{UserId: "bob", Amount: 50, IdempotencyKey: "k1"})
	_, err := client.Withdraw(ctx, &pb.WithdrawRequest{UserId: "bob", Amount: 200, IdempotencyKey: "w1"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v", err)
	}
}

func TestGRPC_UnknownEscrow_StatusCode(t *testing.T) {
	truncateTables(t)
	client, cleanup := newGRPCClient(t)
	defer cleanup()

	_, err := client.ReleaseEscrow(context.Background(), &pb.ReleaseEscrowRequest{OrderId: 9999})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────

func runTestMigrations(dsn string) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db for migrations: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration driver: %v\n", err)
		os.Exit(1)
	}
	_, srcFile, _, _ := runtime.Caller(0)
	migrationsPath := filepath.Join(filepath.Dir(srcFile), "..", "db", "migrations")
	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsPath, "postgres", driver)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration init: %v\n", err)
		os.Exit(1)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		fmt.Fprintf(os.Stderr, "migration up: %v\n", err)
		os.Exit(1)
	}
}
