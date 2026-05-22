package internal

import "context"

type AccountSnapshot struct {
	GrossBalance int64
	Escrowed     int64
	IsReplay     bool
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
	// DepositEscrowed atomically credits gross_balance and escrowed by amount.
	// Funds are visible but not spendable until ConfirmDeposit is called.
	DepositEscrowed(ctx context.Context, userID, depositRef string, amount int64) (AccountSnapshot, error)
	// ConfirmDeposit decrements escrowed by amount, leaving gross_balance unchanged.
	// Available balance increases. Idempotent via "confirm:"+depositRef key.
	ConfirmDeposit(ctx context.Context, userID, depositRef string, amount int64) (AccountSnapshot, error)
}
