package internal

import "context"

type AccountSnapshot struct {
	GrossBalance int64
	Escrowed     int64
}

type InsufficientFundsError struct{ Msg string }
type UnknownUserError struct{ Msg string }
type UnknownEscrowError struct{ Msg string }

func (e *InsufficientFundsError) Error() string { return e.Msg }
func (e *UnknownUserError) Error() string        { return e.Msg }
func (e *UnknownEscrowError) Error() string      { return e.Msg }

type Cashier interface {
	Deposit(ctx context.Context, userID, idempotencyKey string, amount int64) (AccountSnapshot, error)
	Withdraw(ctx context.Context, userID, idempotencyKey string, amount int64) (AccountSnapshot, error)
	Escrow(ctx context.Context, userID string, orderID uint32, amount int64) (AccountSnapshot, error)
	ReleaseEscrow(ctx context.Context, orderID uint32) (AccountSnapshot, error)
	CheckAvailable(ctx context.Context, userID string) (AccountSnapshot, error)
}
