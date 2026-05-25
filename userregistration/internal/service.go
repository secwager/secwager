package internal

import (
	"context"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/secwager/secwager/proto/gen/userregistration"
)

type UserRegistrationService struct {
	pb.UnimplementedUserRegistrationServiceServer
	pool        *pgxpool.Pool
	encryptor   KeyEncryptor
	kmsKeyID    string
	chainParams *chaincfg.Params
}

func NewUserRegistrationService(pool *pgxpool.Pool, enc KeyEncryptor, kmsKeyID string, chainParams *chaincfg.Params) *UserRegistrationService {
	return &UserRegistrationService{pool: pool, encryptor: enc, kmsKeyID: kmsKeyID, chainParams: chainParams}
}

func (s *UserRegistrationService) CompleteRegistration(ctx context.Context, req *pb.CompleteRegistrationRequest) (*pb.CompleteRegistrationResponse, error) {
	if req.CognitoSub == "" {
		return nil, status.Error(codes.InvalidArgument, "cognito_sub is required")
	}
	if req.Username == "" {
		return nil, status.Error(codes.InvalidArgument, "username is required")
	}

	// Idempotency: return existing data if already registered (handles Lambda retries).
	var existingPubkey []byte
	var existingBtcAddr string
	err := s.pool.QueryRow(ctx,
		`SELECT btc_pubkey, btc_addr FROM users WHERE user_id = $1`, req.CognitoSub).
		Scan(&existingPubkey, &existingBtcAddr)
	if err == nil {
		return &pb.CompleteRegistrationResponse{
			UserId: req.CognitoSub, Username: req.Username,
			BtcPubkey: existingPubkey, BtcAddr: existingBtcAddr,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, status.Error(codes.Internal, fmt.Sprintf("idempotency check: %v", err))
	}

	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("generate keypair: %v", err))
	}
	pubKeyBytes := privKey.PubKey().SerializeCompressed()
	privKeyBytes := privKey.Serialize()

	addrHash := btcutil.Hash160(pubKeyBytes)
	btcAddr, err := btcutil.NewAddressWitnessPubKeyHash(addrHash, s.chainParams)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("derive btc address: %v", err))
	}
	btcAddrStr := btcAddr.EncodeAddress()

	encPrivKey, err := s.encryptor.Encrypt(ctx, privKeyBytes)
	for i := range privKeyBytes {
		privKeyBytes[i] = 0
	}
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("encrypt private key: %v", err))
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO users (user_id, username, btc_pubkey, btc_addr, encrypted_privkey, kms_key_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		req.CognitoSub, req.Username, pubKeyBytes, btcAddrStr, encPrivKey, s.kmsKeyID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Concurrent insert race; idempotent — caller gets partial response.
			return &pb.CompleteRegistrationResponse{
				UserId: req.CognitoSub, Username: req.Username, BtcAddr: btcAddrStr,
			}, nil
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("insert user: %v", err))
	}

	return &pb.CompleteRegistrationResponse{
		UserId: req.CognitoSub, Username: req.Username,
		BtcPubkey: pubKeyBytes, BtcAddr: btcAddrStr,
	}, nil
}

func (s *UserRegistrationService) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	if req.Username == "" {
		return nil, status.Error(codes.InvalidArgument, "username is required")
	}
	var userID, username, btcAddr string
	var pubkey []byte
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, username, btc_pubkey, btc_addr FROM users WHERE username = $1`, req.Username).
		Scan(&userID, &username, &pubkey, &btcAddr)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, status.Error(codes.NotFound, "user not found: "+req.Username)
	}
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("get user: %v", err))
	}
	return &pb.GetUserResponse{UserId: userID, Username: username, BtcPubkey: pubkey, BtcAddr: btcAddr}, nil
}
