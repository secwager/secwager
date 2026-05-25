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
// Only valid for tests that don't reach the DB (validation tests).
func newTestService(enc *fakeEncryptor) *UserRegistrationService {
	return &UserRegistrationService{
		pool:        nil,
		encryptor:   enc,
		kmsKeyID:    "fake-kms-key",
		chainParams: &chaincfg.TestNet3Params,
	}
}

func TestCompleteRegistration_RejectsEmptyCognitoSub(t *testing.T) {
	svc := newTestService(&fakeEncryptor{})
	_, err := svc.CompleteRegistration(context.Background(), &pb.CompleteRegistrationRequest{
		CognitoSub: "", Username: "alice", Email: "alice@example.com",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestCompleteRegistration_RejectsEmptyUsername(t *testing.T) {
	svc := newTestService(&fakeEncryptor{})
	_, err := svc.CompleteRegistration(context.Background(), &pb.CompleteRegistrationRequest{
		CognitoSub: "sub-123", Username: "", Email: "alice@example.com",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestGetUser_RejectsEmptyUsername(t *testing.T) {
	svc := newTestService(&fakeEncryptor{})
	_, err := svc.GetUser(context.Background(), &pb.GetUserRequest{Username: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}
