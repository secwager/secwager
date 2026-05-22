package internal

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/secwager/secwager/proto/gen/userregistration"
)

// AlreadyExistsError is returned when the username is already registered.
type AlreadyExistsError struct{ Msg string }

// NotFoundError is returned when the requested user does not exist.
type NotFoundError struct{ Msg string }

func (e *AlreadyExistsError) Error() string { return e.Msg }
func (e *NotFoundError) Error() string      { return e.Msg }

// UserRegistrationService implements pb.UserRegistrationServiceServer.
type UserRegistrationService struct {
	pb.UnimplementedUserRegistrationServiceServer
	pool      *pgxpool.Pool
	users     UserManager
	encryptor KeyEncryptor
	kmsKeyID  string
}

func NewUserRegistrationService(pool *pgxpool.Pool, users UserManager, enc KeyEncryptor, kmsKeyID string) *UserRegistrationService {
	return &UserRegistrationService{pool: pool, users: users, encryptor: enc, kmsKeyID: kmsKeyID}
}

func (s *UserRegistrationService) RegisterUser(ctx context.Context, req *pb.RegisterUserRequest) (*pb.RegisterUserResponse, error) {
	if req.Username == "" {
		return nil, status.Error(codes.InvalidArgument, "username is required")
	}
	if req.Email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}

	// Best-effort early collision check before touching Cognito.
	var existing string
	err := s.pool.QueryRow(ctx, `SELECT user_id FROM users WHERE username = $1`, req.Username).Scan(&existing)
	if err == nil {
		return nil, status.Error(codes.AlreadyExists, "username already registered: "+req.Username)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, status.Error(codes.Internal, fmt.Sprintf("pre-check username: %v", err))
	}

	// Saga step 1: create Cognito user.
	cognitoSub, err := s.users.CreateUser(ctx, req.Username, req.Email)
	if err != nil {
		return nil, toGRPCError(err)
	}

	// Compensating rollback: delete the Cognito user if anything below fails.
	rollback := true
	defer func() {
		if rollback {
			if delErr := s.users.DeleteUser(context.Background(), req.Username); delErr != nil {
				log.Printf("compensating rollback failed for username %s (cognito sub %s): %v", req.Username, cognitoSub, delErr)
			}
		}
	}()

	// Saga step 2: generate secp256k1 keypair.
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("generate keypair: %v", err))
	}
	pubKeyBytes := privKey.PubKey().SerializeCompressed()
	privKeyBytes := privKey.Serialize()

	// Encrypt private key; zero the plaintext immediately after.
	encPrivKey, err := s.encryptor.Encrypt(ctx, privKeyBytes)
	for i := range privKeyBytes {
		privKeyBytes[i] = 0
	}
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("encrypt private key: %v", err))
	}

	// Saga step 3: persist to Postgres.
	_, err = s.pool.Exec(ctx,
		`INSERT INTO users (user_id, username, btc_pubkey, encrypted_privkey, kms_key_id)
		 VALUES ($1, $2, $3, $4, $5)`,
		cognitoSub, req.Username, pubKeyBytes, encPrivKey, s.kmsKeyID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, status.Error(codes.AlreadyExists, "username already registered: "+req.Username)
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("insert user (cognito sub %s): %v", cognitoSub, err))
	}

	rollback = false
	return &pb.RegisterUserResponse{
		UserId:    cognitoSub,
		Username:  req.Username,
		BtcPubkey: pubKeyBytes,
	}, nil
}

func (s *UserRegistrationService) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	if req.Username == "" {
		return nil, status.Error(codes.InvalidArgument, "username is required")
	}
	var userID, username string
	var pubkey []byte
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, username, btc_pubkey FROM users WHERE username = $1`, req.Username).
		Scan(&userID, &username, &pubkey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, status.Error(codes.NotFound, "user not found: "+req.Username)
	}
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("get user: %v", err))
	}
	return &pb.GetUserResponse{UserId: userID, Username: username, BtcPubkey: pubkey}, nil
}

func toGRPCError(err error) error {
	var aee *AlreadyExistsError
	var nfe *NotFoundError
	switch {
	case errors.As(err, &aee):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.As(err, &nfe):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
