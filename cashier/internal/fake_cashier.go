package internal

import (
	"context"
	"fmt"
	"sync"
)

type fakeAccount struct {
	grossBalance int64
	escrowed     int64
}

type fakeEscrow struct {
	userID string
	amount int64
}

// FakeCashier is an in-memory Cashier implementation for unit tests.
type FakeCashier struct {
	mu              sync.Mutex
	accounts        map[string]*fakeAccount
	escrows         map[uint32]fakeEscrow
	idempotencyKeys map[string]struct{}
}

func NewFakeCashier() *FakeCashier {
	return &FakeCashier{
		accounts:        make(map[string]*fakeAccount),
		escrows:         make(map[uint32]fakeEscrow),
		idempotencyKeys: make(map[string]struct{}),
	}
}

func (f *FakeCashier) Deposit(ctx context.Context, userID, idempotencyKey string, amount int64) (AccountSnapshot, error) {
	if amount <= 0 {
		return AccountSnapshot{}, fmt.Errorf("amount must be > 0")
	}
	if idempotencyKey == "" {
		return AccountSnapshot{}, fmt.Errorf("idempotency_key is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, ok := f.idempotencyKeys[idempotencyKey]; ok {
		snap := f.snapshot(f.getOrCreate(userID))
		snap.IsReplay = true
		return snap, nil
	}
	acc := f.getOrCreate(userID)
	acc.grossBalance += amount
	snap := f.snapshot(acc)
	f.idempotencyKeys[idempotencyKey] = struct{}{}
	return snap, nil
}

func (f *FakeCashier) Withdraw(ctx context.Context, userID, idempotencyKey string, amount int64) (AccountSnapshot, error) {
	if amount <= 0 {
		return AccountSnapshot{}, fmt.Errorf("amount must be > 0")
	}
	if idempotencyKey == "" {
		return AccountSnapshot{}, fmt.Errorf("idempotency_key is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, ok := f.idempotencyKeys[idempotencyKey]; ok {
		snap := f.snapshot(f.accounts[userID])
		snap.IsReplay = true
		return snap, nil
	}
	acc, ok := f.accounts[userID]
	if !ok {
		return AccountSnapshot{}, &UnknownUserError{Msg: "user not found: " + userID}
	}
	if acc.grossBalance-acc.escrowed < amount {
		return AccountSnapshot{}, &InsufficientFundsError{
			Msg: fmt.Sprintf("insufficient funds: available=%d requested=%d", acc.grossBalance-acc.escrowed, amount),
		}
	}
	acc.grossBalance -= amount
	snap := f.snapshot(acc)
	f.idempotencyKeys[idempotencyKey] = struct{}{}
	return snap, nil
}

func (f *FakeCashier) Escrow(ctx context.Context, userID string, orderID uint32, amount int64) (AccountSnapshot, error) {
	if amount <= 0 {
		return AccountSnapshot{}, fmt.Errorf("amount must be > 0")
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	// Idempotent: same order_id returns original snapshot.
	if _, exists := f.escrows[orderID]; exists {
		acc := f.accounts[userID]
		return f.snapshot(acc), nil
	}
	acc, ok := f.accounts[userID]
	if !ok {
		return AccountSnapshot{}, &UnknownUserError{Msg: "user not found: " + userID}
	}
	if acc.grossBalance-acc.escrowed < amount {
		return AccountSnapshot{}, &InsufficientFundsError{
			Msg: fmt.Sprintf("insufficient funds: available=%d requested=%d", acc.grossBalance-acc.escrowed, amount),
		}
	}
	acc.escrowed += amount
	f.escrows[orderID] = fakeEscrow{userID: userID, amount: amount}
	return f.snapshot(acc), nil
}

func (f *FakeCashier) ReleaseEscrow(ctx context.Context, orderID uint32) (AccountSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	e, ok := f.escrows[orderID]
	if !ok {
		return AccountSnapshot{}, &UnknownEscrowError{Msg: fmt.Sprintf("escrow not found for order_id: %d", orderID)}
	}
	delete(f.escrows, orderID)
	acc := f.accounts[e.userID]
	acc.escrowed -= e.amount
	return f.snapshot(acc), nil
}

func (f *FakeCashier) CheckAvailable(ctx context.Context, userID string) (AccountSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	acc, ok := f.accounts[userID]
	if !ok {
		return AccountSnapshot{}, nil
	}
	return f.snapshot(acc), nil
}

func (f *FakeCashier) getOrCreate(userID string) *fakeAccount {
	if acc, ok := f.accounts[userID]; ok {
		return acc
	}
	acc := &fakeAccount{}
	f.accounts[userID] = acc
	return acc
}

func (f *FakeCashier) snapshot(acc *fakeAccount) AccountSnapshot {
	return AccountSnapshot{GrossBalance: acc.grossBalance, Escrowed: acc.escrowed}
}
