package internal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

const (
	testUserID   = "user-1"
	testAddr     = "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx" // testnet3 bech32
	testSatoshis = int64(100_000)
)

var testParams = &chaincfg.TestNet3Params

func addrScript(t *testing.T, addrStr string) []byte {
	t.Helper()
	addr, err := btcutil.DecodeAddress(addrStr, testParams)
	if err != nil {
		t.Fatalf("decode address %q: %v", addrStr, err)
	}
	script, err := txscript.PayToAddrScript(addr)
	if err != nil {
		t.Fatalf("pay to addr script: %v", err)
	}
	return script
}

func connectedEvent(height int32, pkScript []byte, satoshis int64) BlockEvent {
	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxOut(&wire.TxOut{Value: satoshis, PkScript: pkScript})
	return BlockEvent{Height: height, Connected: true, RelevantTxs: []*wire.MsgTx{tx}}
}

func testWatcher(node *fakeSPVNode, txs *fakeTxStore2, cashier *fakeCashierClient, users *fakeUserStore, confs int) *Watcher {
	return NewWatcher(node, txs, users, cashier, Config{
		Confirmations: confs,
		PollInterval:  time.Hour,
		NetworkParams: testParams,
	})
}

func runWatcher(t *testing.T, w *Watcher) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	return cancel, done
}

func waitFor(t *testing.T, cond func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestEscrowOnFirstBlock(t *testing.T) {
	node := newFakeSPVNode()
	txs := newFakeTxStore2()
	cashier := newFakeCashierClient()
	users := newFakeUserStore(UserRecord{UserID: testUserID, BtcAddr: testAddr})

	cancel, done := runWatcher(t, testWatcher(node, txs, cashier, users, 6))
	defer cancel()
	time.Sleep(20 * time.Millisecond)

	node.push(connectedEvent(100, addrScript(t, testAddr), testSatoshis))

	waitFor(t, func() bool {
		cashier.mu.Lock()
		defer cashier.mu.Unlock()
		return len(cashier.escrowedCalls) == 1
	}, 2*time.Second)

	cashier.mu.Lock()
	calls := cashier.escrowedCalls
	cashier.mu.Unlock()

	if len(calls) != 1 {
		t.Fatalf("expected 1 DepositEscrowed call, got %d", len(calls))
	}
	if calls[0].UserID != testUserID || calls[0].Satoshis != testSatoshis {
		t.Fatalf("unexpected call: %+v", calls[0])
	}

	waitFor(t, func() bool {
		rows, _ := txs.ListNotEscrowed(context.Background())
		return len(rows) == 0
	}, time.Second)

	cancel()
	<-done
}

func TestConfirmAfterNBlocks(t *testing.T) {
	const confs = 3
	node := newFakeSPVNode()
	txs := newFakeTxStore2()
	cashier := newFakeCashierClient()
	users := newFakeUserStore(UserRecord{UserID: testUserID, BtcAddr: testAddr})

	cancel, done := runWatcher(t, testWatcher(node, txs, cashier, users, confs))
	defer cancel()
	time.Sleep(20 * time.Millisecond)

	const seenHeight = int32(100)
	node.push(connectedEvent(seenHeight, addrScript(t, testAddr), testSatoshis))

	waitFor(t, func() bool {
		cashier.mu.Lock()
		defer cashier.mu.Unlock()
		return len(cashier.escrowedCalls) == 1
	}, 2*time.Second)

	// confs-1 more blocks: no ConfirmDeposit yet.
	for i := 1; i < confs; i++ {
		node.push(BlockEvent{Height: seenHeight + int32(i), Connected: true})
	}
	time.Sleep(50 * time.Millisecond)

	cashier.mu.Lock()
	before := len(cashier.confirmCalls)
	cashier.mu.Unlock()
	if before != 0 {
		t.Fatalf("expected 0 ConfirmDeposit before depth reached, got %d", before)
	}

	// Nth block triggers confirm.
	node.push(BlockEvent{Height: seenHeight + int32(confs), Connected: true})

	waitFor(t, func() bool {
		cashier.mu.Lock()
		defer cashier.mu.Unlock()
		return len(cashier.confirmCalls) == 1
	}, 2*time.Second)

	cashier.mu.Lock()
	n := len(cashier.confirmCalls)
	cashier.mu.Unlock()
	if n != 1 {
		t.Fatalf("expected 1 ConfirmDeposit, got %d", n)
	}

	cancel()
	<-done
}

func TestCashierDown_RetryOnNextBlock(t *testing.T) {
	node := newFakeSPVNode()
	txs := newFakeTxStore2()
	cashier := newFakeCashierClient()
	cashier.escrowErr = errors.New("cashier unavailable")
	users := newFakeUserStore(UserRecord{UserID: testUserID, BtcAddr: testAddr})

	cancel, done := runWatcher(t, testWatcher(node, txs, cashier, users, 6))
	defer cancel()
	time.Sleep(20 * time.Millisecond)

	node.push(connectedEvent(100, addrScript(t, testAddr), testSatoshis))
	time.Sleep(50 * time.Millisecond)

	rows, _ := txs.ListNotEscrowed(context.Background())
	if len(rows) == 0 {
		t.Fatal("expected row to remain unescrowed after cashier failure")
	}

	cashier.escrowErr = nil
	node.push(BlockEvent{Height: 101, Connected: true})

	waitFor(t, func() bool {
		cashier.mu.Lock()
		defer cashier.mu.Unlock()
		return len(cashier.escrowedCalls) == 1
	}, 2*time.Second)

	cancel()
	<-done
}

func TestIdempotency_SameBlockTwice(t *testing.T) {
	node := newFakeSPVNode()
	txs := newFakeTxStore2()
	cashier := newFakeCashierClient()
	users := newFakeUserStore(UserRecord{UserID: testUserID, BtcAddr: testAddr})

	cancel, done := runWatcher(t, testWatcher(node, txs, cashier, users, 6))
	defer cancel()
	time.Sleep(20 * time.Millisecond)

	evt := connectedEvent(100, addrScript(t, testAddr), testSatoshis)
	node.push(evt)
	node.push(evt)

	waitFor(t, func() bool {
		cashier.mu.Lock()
		defer cashier.mu.Unlock()
		return len(cashier.escrowedCalls) >= 1
	}, 2*time.Second)

	time.Sleep(50 * time.Millisecond)

	cashier.mu.Lock()
	n := len(cashier.escrowedCalls)
	cashier.mu.Unlock()
	if n != 1 {
		t.Fatalf("expected 1 DepositEscrowed (idempotent), got %d", n)
	}

	cancel()
	<-done
}

func TestReorg_BlockDisconnected(t *testing.T) {
	node := newFakeSPVNode()
	txs := newFakeTxStore2()
	cashier := newFakeCashierClient()
	cashier.escrowErr = errors.New("blocked")
	users := newFakeUserStore(UserRecord{UserID: testUserID, BtcAddr: testAddr})

	cancel, done := runWatcher(t, testWatcher(node, txs, cashier, users, 6))
	defer cancel()
	time.Sleep(20 * time.Millisecond)

	node.push(connectedEvent(100, addrScript(t, testAddr), testSatoshis))
	time.Sleep(50 * time.Millisecond)

	node.push(BlockEvent{Height: 100, Connected: false})

	waitFor(t, func() bool {
		txs.mu.Lock()
		defer txs.mu.Unlock()
		return len(txs.deposits) == 0
	}, 2*time.Second)

	txs.mu.Lock()
	remaining := len(txs.deposits)
	txs.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("expected 0 deposits after re-org, got %d", remaining)
	}

	cancel()
	<-done
}

func TestNewUser_PickedUpByPoll(t *testing.T) {
	node := newFakeSPVNode()
	txs := newFakeTxStore2()
	cashier := newFakeCashierClient()
	users := newFakeUserStore()

	w := NewWatcher(node, txs, users, cashier, Config{
		Confirmations: 6,
		PollInterval:  30 * time.Millisecond,
		NetworkParams: testParams,
	})
	cancel, done := runWatcher(t, w)
	defer cancel()

	time.Sleep(20 * time.Millisecond)
	users.add(UserRecord{UserID: testUserID, BtcAddr: testAddr})
	time.Sleep(80 * time.Millisecond) // wait for poll

	node.push(connectedEvent(200, addrScript(t, testAddr), testSatoshis))

	waitFor(t, func() bool {
		cashier.mu.Lock()
		defer cashier.mu.Unlock()
		return len(cashier.escrowedCalls) == 1
	}, 2*time.Second)

	cancel()
	<-done
}

func TestMultipleDeposits_SameAddress(t *testing.T) {
	node := newFakeSPVNode()
	txs := newFakeTxStore2()
	cashier := newFakeCashierClient()
	users := newFakeUserStore(UserRecord{UserID: testUserID, BtcAddr: testAddr})

	cancel, done := runWatcher(t, testWatcher(node, txs, cashier, users, 6))
	defer cancel()
	time.Sleep(20 * time.Millisecond)

	pkScript := addrScript(t, testAddr)

	tx1 := wire.NewMsgTx(wire.TxVersion)
	tx1.AddTxOut(&wire.TxOut{Value: 50_000, PkScript: pkScript})

	tx2 := wire.NewMsgTx(wire.TxVersion)
	tx2.AddTxIn(&wire.TxIn{}) // makes tx2 hash differ from tx1
	tx2.AddTxOut(&wire.TxOut{Value: 75_000, PkScript: pkScript})

	node.push(BlockEvent{Height: 300, Connected: true, RelevantTxs: []*wire.MsgTx{tx1, tx2}})

	waitFor(t, func() bool {
		cashier.mu.Lock()
		defer cashier.mu.Unlock()
		return len(cashier.escrowedCalls) == 2
	}, 2*time.Second)

	cashier.mu.Lock()
	n := len(cashier.escrowedCalls)
	cashier.mu.Unlock()
	if n != 2 {
		t.Fatalf("expected 2 DepositEscrowed calls, got %d", n)
	}

	cancel()
	<-done
}
