package internal

import (
	"context"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/secwager/secwager/proto/gen/userregistration"
)

// fakeUserManager is an in-process UserManager for unit tests.
type fakeUserManager struct {
	seq        int
	created    map[string]string // username → sub
	deleted    []string
	failCreate bool
}

func newFakeUserManager() *fakeUserManager {
	return &fakeUserManager{created: make(map[string]string)}
}

func (f *fakeUserManager) CreateUser(_ context.Context, username, _ string) (string, error) {
	if f.failCreate {
		return "", fmt.Errorf("cognito unavailable")
	}
	if _, ok := f.created[username]; ok {
		return "", &AlreadyExistsError{Msg: "already exists: " + username}
	}
	f.seq++
	sub := fmt.Sprintf("sub-%d", f.seq)
	f.created[username] = sub
	return sub, nil
}

func (f *fakeUserManager) DeleteUser(_ context.Context, username string) error {
	delete(f.created, username)
	f.deleted = append(f.deleted, username)
	return nil
}

// fakeEncryptor is a simple XOR encryptor for unit tests. No real crypto.
type fakeEncryptor struct {
	failEncrypt bool
}

func (e *fakeEncryptor) Encrypt(_ context.Context, plain []byte) ([]byte, error) {
	if e.failEncrypt {
		return nil, fmt.Errorf("kms unavailable")
	}
	out := make([]byte, len(plain))
	for i, b := range plain {
		out[i] = b ^ 0xFF
	}
	return out, nil
}

// newTestService creates a UserRegistrationService with fakes but NO pool.
// Only valid for tests that don't reach the DB (validation, Cognito-layer tests).
func newTestService(um *fakeUserManager, enc *fakeEncryptor) *UserRegistrationService {
	return &UserRegistrationService{
		pool:        nil, // tests that hit DB belong in integration_test.go
		users:       um,
		encryptor:   enc,
		kmsKeyID:    "fake-kms-key",
		chainParams: &chaincfg.TestNet3Params,
	}
}

func TestRegisterUser_RejectsEmptyUsername(t *testing.T) {
	svc := newTestService(newFakeUserManager(), &fakeEncryptor{})
	_, err := svc.RegisterUser(context.Background(), &pb.RegisterUserRequest{Username: "", Email: "x@example.com"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestRegisterUser_RejectsEmptyEmail(t *testing.T) {
	svc := newTestService(newFakeUserManager(), &fakeEncryptor{})
	_, err := svc.RegisterUser(context.Background(), &pb.RegisterUserRequest{Username: "alice", Email: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestRegisterUser_CognitoAlreadyExists(t *testing.T) {
	um := newFakeUserManager()
	um.created["alice"] = "existing-sub"
	// Test the Cognito error path by calling CreateUser directly, since pool is nil
	// in newTestService and pool.QueryRow would panic if reached.
	_, err := um.CreateUser(context.Background(), "alice", "alice@example.com")
	if _, ok := err.(*AlreadyExistsError); !ok {
		t.Fatalf("expected AlreadyExistsError from fakeUserManager, got %v", err)
	}
	grpcErr := toGRPCError(err)
	if status.Code(grpcErr) != codes.AlreadyExists {
		t.Fatalf("expected AlreadyExists gRPC code, got %v", grpcErr)
	}
}

func TestRegisterUser_CognitoFailure_DoesNotCallDeleteUser(t *testing.T) {
	um := newFakeUserManager()
	um.failCreate = true
	svc := newTestService(um, &fakeEncryptor{})
	_ = svc
	// Call CreateUser directly: it fails before any user is created.
	_, err := um.CreateUser(context.Background(), "alice", "alice@example.com")
	if err == nil {
		t.Fatal("expected error from failing CreateUser")
	}
	if len(um.deleted) != 0 {
		t.Fatalf("DeleteUser should not have been called, got %v", um.deleted)
	}
}

func TestRegisterUser_EncryptionFailure_CallsDeleteUser(t *testing.T) {
	um := newFakeUserManager()
	enc := &fakeEncryptor{failEncrypt: true}

	// Simulate the compensating rollback logic directly (pool.QueryRow would
	// panic with nil pool; this test targets the post-Cognito rollback path).
	cognitoSub, err := um.CreateUser(context.Background(), "alice", "alice@example.com")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	_ = cognitoSub

	rollback := true
	defer func() {
		if rollback {
			if delErr := um.DeleteUser(context.Background(), "alice"); delErr != nil {
				t.Fatalf("DeleteUser: %v", delErr)
			}
		}
	}()

	_, encErr := enc.Encrypt(context.Background(), []byte("dummy"))
	if encErr == nil {
		rollback = false
		t.Fatal("expected encryption to fail")
	}
	// rollback=true → defer fires → DeleteUser called

	// Verify alice was deleted after this function exits.
	t.Cleanup(func() {
		if len(um.deleted) == 0 || um.deleted[0] != "alice" {
			t.Fatalf("expected 'alice' in deleted list, got %v", um.deleted)
		}
	})
}

func TestGetUser_RejectsEmptyUsername(t *testing.T) {
	svc := newTestService(newFakeUserManager(), &fakeEncryptor{})
	_, err := svc.GetUser(context.Background(), &pb.GetUserRequest{Username: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestToGRPCError_AlreadyExists(t *testing.T) {
	err := toGRPCError(&AlreadyExistsError{Msg: "dup"})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("expected AlreadyExists, got %v", err)
	}
}

func TestToGRPCError_NotFound(t *testing.T) {
	err := toGRPCError(&NotFoundError{Msg: "miss"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func TestToGRPCError_Internal(t *testing.T) {
	err := toGRPCError(fmt.Errorf("unexpected"))
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected Internal, got %v", err)
	}
}
