package internal

import (
	"context"
	"errors"
	"testing"
)

func setup() (Cashier, context.Context) {
	return NewFakeCashier(), context.Background()
}

func TestDeposit_NewUser(t *testing.T) {
	c, ctx := setup()
	snap, err := c.Deposit(ctx, "alice", "key-1", 100)
	if err != nil {
		t.Fatal(err)
	}
	if snap.GrossBalance != 100 || snap.Escrowed != 0 {
		t.Fatalf("got %+v", snap)
	}
}

func TestDeposit_ExistingUser(t *testing.T) {
	c, ctx := setup()
	c.Deposit(ctx, "alice", "key-1", 100)
	snap, err := c.Deposit(ctx, "alice", "key-2", 50)
	if err != nil {
		t.Fatal(err)
	}
	if snap.GrossBalance != 150 {
		t.Fatalf("expected 150, got %d", snap.GrossBalance)
	}
}

func TestDeposit_RejectsNonPositiveAmount(t *testing.T) {
	c, ctx := setup()
	if _, err := c.Deposit(ctx, "alice", "key-1", 0); err == nil {
		t.Fatal("expected error for zero amount")
	}
	if _, err := c.Deposit(ctx, "alice", "key-2", -1); err == nil {
		t.Fatal("expected error for negative amount")
	}
}

func TestDeposit_Idempotent(t *testing.T) {
	c, ctx := setup()
	snap1, _ := c.Deposit(ctx, "alice", "key-1", 100)
	snap2, _ := c.Deposit(ctx, "alice", "key-1", 100) // same key
	if snap1.GrossBalance != snap2.GrossBalance || snap1.Escrowed != snap2.Escrowed {
		t.Fatalf("idempotency broken: got %+v then %+v", snap1, snap2)
	}
	if snap1.IsReplay {
		t.Fatal("first call should not be a replay")
	}
	if !snap2.IsReplay {
		t.Fatal("second call should be a replay")
	}
	// Balance should only be 100, not 200.
	check, _ := c.CheckAvailable(ctx, "alice")
	if check.GrossBalance != 100 {
		t.Fatalf("double-applied deposit, got gross_balance=%d", check.GrossBalance)
	}
}

func TestWithdraw_SufficientFunds(t *testing.T) {
	c, ctx := setup()
	c.Deposit(ctx, "alice", "dep-1", 100)
	snap, err := c.Withdraw(ctx, "alice", "wth-1", 40)
	if err != nil {
		t.Fatal(err)
	}
	if snap.GrossBalance != 60 {
		t.Fatalf("expected 60, got %d", snap.GrossBalance)
	}
}

func TestWithdraw_InsufficientFunds(t *testing.T) {
	c, ctx := setup()
	c.Deposit(ctx, "alice", "dep-1", 50)
	_, err := c.Withdraw(ctx, "alice", "wth-1", 100)
	var ife *InsufficientFundsError
	if !errors.As(err, &ife) {
		t.Fatalf("expected InsufficientFundsError, got %v", err)
	}
}

func TestWithdraw_UnknownUser(t *testing.T) {
	c, ctx := setup()
	_, err := c.Withdraw(ctx, "nobody", "wth-1", 10)
	var uue *UnknownUserError
	if !errors.As(err, &uue) {
		t.Fatalf("expected UnknownUserError, got %v", err)
	}
}

func TestWithdraw_Idempotent(t *testing.T) {
	c, ctx := setup()
	c.Deposit(ctx, "alice", "dep-1", 100)
	snap1, _ := c.Withdraw(ctx, "alice", "wth-1", 30)
	snap2, _ := c.Withdraw(ctx, "alice", "wth-1", 30) // same key
	if snap1.GrossBalance != snap2.GrossBalance || snap1.Escrowed != snap2.Escrowed {
		t.Fatalf("idempotency broken: %+v vs %+v", snap1, snap2)
	}
	if !snap2.IsReplay {
		t.Fatal("second call should be a replay")
	}
	check, _ := c.CheckAvailable(ctx, "alice")
	if check.GrossBalance != 70 {
		t.Fatalf("double-applied withdrawal, gross_balance=%d", check.GrossBalance)
	}
}

func TestEscrow_LocksFunds(t *testing.T) {
	c, ctx := setup()
	c.Deposit(ctx, "alice", "dep-1", 100)
	snap, err := c.Escrow(ctx, "alice", 1, 60)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Escrowed != 60 || snap.GrossBalance != 100 {
		t.Fatalf("got %+v", snap)
	}
}

func TestEscrow_BlocksWithdrawalBeyondAvailable(t *testing.T) {
	c, ctx := setup()
	c.Deposit(ctx, "alice", "dep-1", 100)
	c.Escrow(ctx, "alice", 1, 80)
	_, err := c.Withdraw(ctx, "alice", "wth-1", 30)
	var ife *InsufficientFundsError
	if !errors.As(err, &ife) {
		t.Fatalf("expected InsufficientFundsError, got %v", err)
	}
}

func TestEscrow_InsufficientFunds(t *testing.T) {
	c, ctx := setup()
	c.Deposit(ctx, "alice", "dep-1", 50)
	_, err := c.Escrow(ctx, "alice", 1, 100)
	var ife *InsufficientFundsError
	if !errors.As(err, &ife) {
		t.Fatalf("expected InsufficientFundsError, got %v", err)
	}
}

func TestEscrow_Idempotent(t *testing.T) {
	c, ctx := setup()
	c.Deposit(ctx, "alice", "dep-1", 100)
	snap1, _ := c.Escrow(ctx, "alice", 42, 50)
	snap2, _ := c.Escrow(ctx, "alice", 42, 50) // same order_id
	if snap1 != snap2 {
		t.Fatalf("idempotency broken: %+v vs %+v", snap1, snap2)
	}
	check, _ := c.CheckAvailable(ctx, "alice")
	if check.Escrowed != 50 {
		t.Fatalf("double-applied escrow, escrowed=%d", check.Escrowed)
	}
}

func TestEscrow_MultipleOrders(t *testing.T) {
	c, ctx := setup()
	c.Deposit(ctx, "alice", "dep-1", 100)
	c.Escrow(ctx, "alice", 1, 30)
	c.Escrow(ctx, "alice", 2, 30)
	snap, err := c.Escrow(ctx, "alice", 3, 30)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Escrowed != 90 {
		t.Fatalf("expected escrowed=90, got %d", snap.Escrowed)
	}
}

func TestReleaseEscrow_RestoresFunds(t *testing.T) {
	c, ctx := setup()
	c.Deposit(ctx, "alice", "dep-1", 100)
	c.Escrow(ctx, "alice", 1, 60)
	snap, err := c.ReleaseEscrow(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Escrowed != 0 || snap.GrossBalance != 100 {
		t.Fatalf("got %+v", snap)
	}
}

func TestReleaseEscrow_UnknownEscrow(t *testing.T) {
	c, ctx := setup()
	_, err := c.ReleaseEscrow(ctx, 999)
	var uee *UnknownEscrowError
	if !errors.As(err, &uee) {
		t.Fatalf("expected UnknownEscrowError, got %v", err)
	}
}

func TestCheckAvailable_UnknownUserReturnsZero(t *testing.T) {
	c, ctx := setup()
	snap, err := c.CheckAvailable(ctx, "nobody")
	if err != nil {
		t.Fatal(err)
	}
	if snap.GrossBalance != 0 || snap.Escrowed != 0 {
		t.Fatalf("expected zero snapshot, got %+v", snap)
	}
}

func TestDepositEscrowed_FundsVisibleButLocked(t *testing.T) {
	c, ctx := setup()
	snap, err := c.DepositEscrowed(ctx, "alice", "abc123:0", 500)
	if err != nil {
		t.Fatal(err)
	}
	if snap.GrossBalance != 500 || snap.Escrowed != 500 {
		t.Fatalf("expected gross=500 escrowed=500, got %+v", snap)
	}
	// available = gross - escrowed = 0
	avail, _ := c.CheckAvailable(ctx, "alice")
	if avail.GrossBalance-avail.Escrowed != 0 {
		t.Fatalf("expected available=0, got %d", avail.GrossBalance-avail.Escrowed)
	}
}

func TestDepositEscrowed_Idempotent(t *testing.T) {
	c, ctx := setup()
	snap1, _ := c.DepositEscrowed(ctx, "alice", "abc123:0", 500)
	snap2, _ := c.DepositEscrowed(ctx, "alice", "abc123:0", 500)
	if snap1.GrossBalance != snap2.GrossBalance {
		t.Fatalf("idempotency broken: %+v vs %+v", snap1, snap2)
	}
	if !snap2.IsReplay {
		t.Fatal("second call should be a replay")
	}
	check, _ := c.CheckAvailable(ctx, "alice")
	if check.GrossBalance != 500 {
		t.Fatalf("double-applied deposit, got gross_balance=%d", check.GrossBalance)
	}
}

func TestConfirmDeposit_MovesToAvailable(t *testing.T) {
	c, ctx := setup()
	c.DepositEscrowed(ctx, "alice", "abc123:0", 500)
	snap, err := c.ConfirmDeposit(ctx, "alice", "abc123:0", 500)
	if err != nil {
		t.Fatal(err)
	}
	// escrowed decreases, gross unchanged → available = 500
	if snap.GrossBalance != 500 || snap.Escrowed != 0 {
		t.Fatalf("expected gross=500 escrowed=0, got %+v", snap)
	}
}

func TestConfirmDeposit_Idempotent(t *testing.T) {
	c, ctx := setup()
	c.DepositEscrowed(ctx, "alice", "abc123:0", 500)
	snap1, _ := c.ConfirmDeposit(ctx, "alice", "abc123:0", 500)
	snap2, _ := c.ConfirmDeposit(ctx, "alice", "abc123:0", 500)
	if snap1.GrossBalance != snap2.GrossBalance {
		t.Fatalf("idempotency broken: %+v vs %+v", snap1, snap2)
	}
	if !snap2.IsReplay {
		t.Fatal("second ConfirmDeposit should be a replay")
	}
}

func TestDepositEscrowed_ThenOrderEscrow_Available(t *testing.T) {
	c, ctx := setup()
	// BTC deposit in escrow
	c.DepositEscrowed(ctx, "alice", "abc123:0", 1000)
	// Confirm deposit → available = 1000
	c.ConfirmDeposit(ctx, "alice", "abc123:0", 1000)
	// Place order escrow
	snap, err := c.Escrow(ctx, "alice", 7, 400)
	if err != nil {
		t.Fatal(err)
	}
	if snap.GrossBalance != 1000 || snap.Escrowed != 400 {
		t.Fatalf("expected gross=1000 escrowed=400, got %+v", snap)
	}
}
