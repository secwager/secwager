package internal

import (
	"context"

	pb "github.com/secwager/secwager/proto/gen/cashier"
	"google.golang.org/grpc"
)

// CashierClient abstracts the cashier gRPC deposit operations.
type CashierClient interface {
	DepositEscrowed(ctx context.Context, userID string, satoshis int64, depositRef string) error
	ConfirmDeposit(ctx context.Context, userID string, satoshis int64, depositRef string) error
}

type grpcCashierClient struct {
	client pb.CashierServiceClient
}

// NewGRPCCashierClient wraps a gRPC CashierServiceClient.
func NewGRPCCashierClient(conn *grpc.ClientConn) CashierClient {
	return &grpcCashierClient{client: pb.NewCashierServiceClient(conn)}
}

func (c *grpcCashierClient) DepositEscrowed(ctx context.Context, userID string, satoshis int64, depositRef string) error {
	_, err := c.client.DepositEscrowed(ctx, &pb.DepositEscrowedRequest{
		UserId:     userID,
		Amount:     satoshis,
		DepositRef: depositRef,
	})
	return err
}

func (c *grpcCashierClient) ConfirmDeposit(ctx context.Context, userID string, satoshis int64, depositRef string) error {
	_, err := c.client.ConfirmDeposit(ctx, &pb.ConfirmDepositRequest{
		UserId:     userID,
		Amount:     satoshis,
		DepositRef: depositRef,
	})
	return err
}
