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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/secwager/secwager/proto/gen/userregistration"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "userreg",
			"POSTGRES_PASSWORD": "userreg",
			"POSTGRES_DB":       "userreg",
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
	dsn := fmt.Sprintf("host=%s port=%s user=userreg password=userreg dbname=userreg sslmode=disable", host, port.Port())

	runTestMigrations(dsn)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create pool: %v\n", err)
		os.Exit(1)
	}
	testPool = pool
	defer pool.Close()

	os.Exit(m.Run())
}

func truncate(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `TRUNCATE TABLE users`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func newTestSvc() *UserRegistrationService {
	return NewUserRegistrationService(testPool, newFakeUserManager(), &fakeEncryptor{}, "fake-kms-key")
}

// ── DB-backed tests ───────────────────────────────────────────────────────────

func TestDB_RegisterUser_Success(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	svc := newTestSvc()
	resp, err := svc.RegisterUser(ctx, &pb.RegisterUserRequest{Username: "alice", Email: "alice@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Username != "alice" || resp.UserId == "" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if len(resp.BtcPubkey) != 33 {
		t.Fatalf("expected 33-byte compressed pubkey, got %d bytes", len(resp.BtcPubkey))
	}

	// Verify row persisted correctly.
	var dbUserID string
	var dbPubkey, dbEncPrivKey []byte
	err = testPool.QueryRow(ctx,
		`SELECT user_id, btc_pubkey, encrypted_privkey FROM users WHERE username = 'alice'`).
		Scan(&dbUserID, &dbPubkey, &dbEncPrivKey)
	if err != nil {
		t.Fatalf("db check: %v", err)
	}
	if dbUserID != resp.UserId {
		t.Fatalf("user_id mismatch: resp=%s db=%s", resp.UserId, dbUserID)
	}
	if string(dbPubkey) != string(resp.BtcPubkey) {
		t.Fatal("btc_pubkey mismatch between response and DB")
	}
	if len(dbEncPrivKey) == 0 {
		t.Fatal("encrypted_privkey should be non-empty in DB")
	}
}

func TestDB_RegisterUser_AlreadyExists(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	svc := newTestSvc()
	svc.RegisterUser(ctx, &pb.RegisterUserRequest{Username: "bob", Email: "bob@example.com"})

	// Use a fresh service so fakeUserManager doesn't block on duplicate.
	svc2 := newTestSvc()
	_, err := svc2.RegisterUser(ctx, &pb.RegisterUserRequest{Username: "bob", Email: "bob2@example.com"})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("expected AlreadyExists, got %v", err)
	}
}

func TestDB_GetUser_NotFound(t *testing.T) {
	truncate(t)
	svc := newTestSvc()
	_, err := svc.GetUser(context.Background(), &pb.GetUserRequest{Username: "ghost"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func TestDB_GetUser_Found(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	svc := newTestSvc()
	created, err := svc.RegisterUser(ctx, &pb.RegisterUserRequest{Username: "carol", Email: "carol@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetUser(ctx, &pb.GetUserRequest{Username: "carol"})
	if err != nil {
		t.Fatal(err)
	}
	if got.UserId != created.UserId {
		t.Fatalf("user_id mismatch: created=%s got=%s", created.UserId, got.UserId)
	}
	if string(got.BtcPubkey) != string(created.BtcPubkey) {
		t.Fatal("btc_pubkey mismatch between RegisterUser and GetUser")
	}
}

// ── gRPC handler tests ────────────────────────────────────────────────────────

func newGRPCClient(t *testing.T) (pb.UserRegistrationServiceClient, func()) {
	t.Helper()
	const bufSize = 1 << 20
	lis := bufconn.Listen(bufSize)

	srv := grpc.NewServer()
	pb.RegisterUserRegistrationServiceServer(srv, newTestSvc())
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
	return pb.NewUserRegistrationServiceClient(conn), func() {
		conn.Close()
		srv.GracefulStop()
	}
}

func TestGRPC_RegisterUser_ResponseAndDB(t *testing.T) {
	truncate(t)
	client, cleanup := newGRPCClient(t)
	defer cleanup()
	ctx := context.Background()

	resp, err := client.RegisterUser(ctx, &pb.RegisterUserRequest{Username: "dave", Email: "dave@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Username != "dave" || resp.UserId == "" || len(resp.BtcPubkey) != 33 {
		t.Fatalf("unexpected response: %+v", resp)
	}

	var dbUserID string
	testPool.QueryRow(ctx, `SELECT user_id FROM users WHERE username = 'dave'`).Scan(&dbUserID)
	if dbUserID != resp.UserId {
		t.Fatalf("DB user_id mismatch: resp=%s db=%s", resp.UserId, dbUserID)
	}
}

func TestGRPC_RegisterUser_AlreadyExists_StatusCode(t *testing.T) {
	truncate(t)
	client, cleanup := newGRPCClient(t)
	defer cleanup()
	ctx := context.Background()

	client.RegisterUser(ctx, &pb.RegisterUserRequest{Username: "eve", Email: "eve@example.com"})
	// Second call hits DB pre-check (same pool, row exists).
	_, err := client.RegisterUser(ctx, &pb.RegisterUserRequest{Username: "eve", Email: "eve2@example.com"})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("expected AlreadyExists, got %v", err)
	}
}

func TestGRPC_GetUser_NotFound_StatusCode(t *testing.T) {
	truncate(t)
	client, cleanup := newGRPCClient(t)
	defer cleanup()

	_, err := client.GetUser(context.Background(), &pb.GetUserRequest{Username: "nobody"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func TestGRPC_RegisterUser_EmptyUsername_StatusCode(t *testing.T) {
	truncate(t)
	client, cleanup := newGRPCClient(t)
	defer cleanup()

	_, err := client.RegisterUser(context.Background(), &pb.RegisterUserRequest{Username: "", Email: "x@example.com"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

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
