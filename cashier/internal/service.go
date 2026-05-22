package internal

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/secwager/secwager/cashier/gen/cashier"
)

type CashierService struct {
	pb.UnimplementedCashierServiceServer
	cashier Cashier
}

func NewCashierService(c Cashier) *CashierService {
	return &CashierService{cashier: c}
}

func (s *CashierService) Deposit(ctx context.Context, req *pb.DepositRequest) (*pb.CashierResponse, error) {
	if req.Amount <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be > 0")
	}
	if req.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}
	snap, err := s.cashier.Deposit(ctx, req.UserId, req.IdempotencyKey, req.Amount)
	return toResponse(snap, err)
}

func (s *CashierService) Withdraw(ctx context.Context, req *pb.WithdrawRequest) (*pb.CashierResponse, error) {
	if req.Amount <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be > 0")
	}
	if req.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}
	snap, err := s.cashier.Withdraw(ctx, req.UserId, req.IdempotencyKey, req.Amount)
	return toResponse(snap, err)
}

func (s *CashierService) Escrow(ctx context.Context, req *pb.EscrowRequest) (*pb.CashierResponse, error) {
	if req.Amount <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be > 0")
	}
	snap, err := s.cashier.Escrow(ctx, req.UserId, req.OrderId, req.Amount)
	return toResponse(snap, err)
}

func (s *CashierService) ReleaseEscrow(ctx context.Context, req *pb.ReleaseEscrowRequest) (*pb.CashierResponse, error) {
	snap, err := s.cashier.ReleaseEscrow(ctx, req.OrderId)
	return toResponse(snap, err)
}

func (s *CashierService) CheckAvailable(ctx context.Context, req *pb.CheckRequest) (*pb.CashierResponse, error) {
	snap, err := s.cashier.CheckAvailable(ctx, req.UserId)
	return toResponse(snap, err)
}

func toResponse(snap AccountSnapshot, err error) (*pb.CashierResponse, error) {
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &pb.CashierResponse{
		GrossBalance: snap.GrossBalance,
		Escrowed:     snap.Escrowed,
	}, nil
}

func toGRPCError(err error) error {
	var ife *InsufficientFundsError
	var uue *UnknownUserError
	var uee *UnknownEscrowError
	switch {
	case errors.As(err, &ife):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.As(err, &uue):
		return status.Error(codes.NotFound, err.Error())
	case errors.As(err, &uee):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
