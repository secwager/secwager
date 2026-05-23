//go:build integration

package internal

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"strings"
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

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btclog"
	"github.com/lightninglabs/neutrino"

	pb "github.com/secwager/secwager/proto/gen/cashier"
)

const (
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

// testLogWriter routes an io.Writer into t.Log, so btclog output
// obeys normal go test rules (shown with -v, suppressed on pass).
type testLogWriter struct{ t *testing.T }

func (w *testLogWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
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

// scrapedTx holds the details of a real testnet transaction extracted during the scrape phase.
type scrapedTx struct {
	addr        string
	txid        string
	vout        int
	satoshis    int64
	blockHeight int32
}

// scrapeTransaction waits for neutrino to reach the chain tip, then walks back
// a few blocks to find the first output with a parseable address. No private
// key or faucet required — we just borrow a random on-chain output as our test
// deposit target.
func scrapeTransaction(t *testing.T, ctx context.Context, node *neutrinoSPVNode, params *chaincfg.Params) scrapedTx {
	t.Helper()

	// Route neutrino's internal btclog output through t.Log so it obeys
	// normal go test rules: visible with -v, suppressed on pass.
	ntrLogger := btclog.NewBackend(&testLogWriter{t}).Logger("NTRN")
	ntrLogger.SetLevel(btclog.LevelInfo)
	neutrino.UseLogger(ntrLogger)

	t.Log("scrape: waiting for neutrino header sync...")

	// Print best-block height every 5 s so progress is visible.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				bs, err := node.cs.BestBlock()
				if err == nil {
					t.Logf("sync: height=%d current=%v", bs.Height, node.cs.IsCurrent())
				}
			}
		}
	}()

	for !node.cs.IsCurrent() {
		select {
		case <-ctx.Done():
			t.Fatal("context expired waiting for header sync")
		case <-time.After(5 * time.Second):
		}
	}

	bs, err := node.cs.BestBlock()
	if err != nil {
		t.Fatalf("scrape: BestBlock: %v", err)
	}
	t.Logf("scrape: synced to height %d", bs.Height)

	// Try blocks from tip-2 backward until we find a usable output.
	for delta := int32(2); delta <= 20; delta++ {
		targetHeight := bs.Height - delta
		if targetHeight <= 0 {
			break
		}

		header, err := node.cs.BlockHeaders.FetchHeaderByHeight(uint32(targetHeight))
		if err != nil {
			t.Logf("scrape: FetchHeaderByHeight(%d): %v — skipping", targetHeight, err)
			continue
		}
		hash := header.BlockHash()

		block, err := node.cs.GetBlock(hash)
		if err != nil {
			t.Logf("scrape: GetBlock at %d: %v — skipping", targetHeight, err)
			continue
		}

		for _, tx := range block.Transactions() {
			for vout, out := range tx.MsgTx().TxOut {
				addr, err := outputAddress(out.PkScript, params)
				if err != nil || out.Value <= 0 {
					continue
				}
				return scrapedTx{
					addr:        addr,
					txid:        tx.Hash().String(),
					vout:        vout,
					satoshis:    out.Value,
					blockHeight: targetHeight,
				}
			}
		}
		t.Logf("scrape: no usable output in block %d, trying one block earlier", targetHeight)
	}

	t.Fatal("scrape: no addressable output found in any of the tried blocks")
	return scrapedTx{}
}

// TestIntegration_DepositEscrowAndConfirm runs a neutrino node on testnet3,
// scrapes a real confirmed transaction from near the chain tip, then replays
// the watcher from just before that block to assert DepositEscrowed and
// ConfirmDeposit are called in order.
func TestIntegration_DepositEscrowAndConfirm(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
	defer cancel()

	migDir := migrationsPath(t)
	dsn := startPostgres(t, ctx, "btcwatcher", "btcwatcher", "btcwatcher")
	runMigrationsForTest(t, dsn, migDir)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	if err := os.MkdirAll(integTestDataDir, 0755); err != nil {
		t.Fatalf("mkdir dataDir: %v", err)
	}
	params := &chaincfg.TestNet3Params

	node, err := NewNeutrinoSPVNode(integTestDataDir, params, nil)
	if err != nil {
		t.Fatalf("neutrino node: %v", err)
	}
	defer node.Stop()

	// Phase 1: scrape a real transaction from near the current chain tip.
	nnode := node.(*neutrinoSPVNode)
	found := scrapeTransaction(t, ctx, nnode, params)
	t.Logf("scraped: txid=%s vout=%d satoshis=%d height=%d addr=%s",
		found.txid, found.vout, found.satoshis, found.blockHeight, found.addr)

	// Phase 2: run the watcher starting from one block before the found tx.
	// The watcher will scan the handful of blocks from startHeight to tip,
	// triggering DepositEscrowed and ConfirmDeposit during initial catch-up.

	userStore := newFakeUserStore(UserRecord{
		UserID:  integTestUserID,
		BtcAddr: found.addr,
	})

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

	w := NewWatcher(
		node,
		NewPostgresTxStore(pool),
		userStore,
		NewGRPCCashierClient(conn),
		Config{
			Confirmations: 1,
			PollInterval:  time.Minute,
			NetworkParams: params,
			StartHeight:   found.blockHeight - 1,
		},
	)

	watcherDone := make(chan error, 1)
	go func() { watcherDone <- w.Run(ctx) }()

	// Both assertions should fire during the initial catch-up scan.
	// The scraped tx has at least 2 confirmations already, so no live blocks needed.
	waitFor(t, func() bool {
		fakeCashier.mu.Lock()
		defer fakeCashier.mu.Unlock()
		return len(fakeCashier.escrowedCalls) > 0
	}, 10*time.Minute)

	fakeCashier.mu.Lock()
	ec := len(fakeCashier.escrowedCalls)
	fakeCashier.mu.Unlock()
	if ec == 0 {
		t.Fatal("DepositEscrowed was never called")
	}
	t.Logf("DepositEscrowed called %d time(s)", ec)

	waitFor(t, func() bool {
		fakeCashier.mu.Lock()
		defer fakeCashier.mu.Unlock()
		return len(fakeCashier.confirmCalls) > 0
	}, 10*time.Minute)

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
