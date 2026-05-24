package internal

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
)

// fakeSPVNode drives block events from a test-controlled channel.
type fakeSPVNode struct {
	mu        sync.Mutex
	addrs     []btcutil.Address
	blockFeed chan BlockEvent
}

func newFakeSPVNode() *fakeSPVNode {
	return &fakeSPVNode{blockFeed: make(chan BlockEvent, 64)}
}

func (f *fakeSPVNode) WatchAddresses(addrs []btcutil.Address) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addrs = append(f.addrs, addrs...)
	return nil
}

func (f *fakeSPVNode) Blocks(ctx context.Context, _ int32) (<-chan BlockEvent, error) {
	ch := make(chan BlockEvent, 64)
	go func() {
		defer close(ch)
		for {
			select {
			case evt, ok := <-f.blockFeed:
				if !ok {
					return
				}
				select {
				case ch <- evt:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

func (f *fakeSPVNode) BestBlock(_ context.Context) (int32, error) { return 0, nil }
func (f *fakeSPVNode) Stop() error                                 { return nil }
func (f *fakeSPVNode) push(evt BlockEvent)                         { f.blockFeed <- evt }

// fakeDepositState tracks escrowed, confirmed, and reorg state for a deposit.
type fakeDepositState struct {
	Deposit
	confirmed bool
	reorgedAt *time.Time
}

// fakeTxStore2 is an in-memory TxStore.
type fakeTxStore2 struct {
	mu       sync.Mutex
	deposits map[string]*fakeDepositState
}

func newFakeTxStore2() *fakeTxStore2 {
	return &fakeTxStore2{deposits: make(map[string]*fakeDepositState)}
}

func (s *fakeTxStore2) key(txid string, vout int) string {
	return txid + ":" + strconv.Itoa(vout)
}

func (s *fakeTxStore2) Insert(_ context.Context, d Deposit) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := s.key(d.Txid, d.Vout)
	if existing, exists := s.deposits[k]; exists {
		// Un-reorg if the txn reappears.
		if existing.reorgedAt != nil {
			existing.SeenAtHeight = d.SeenAtHeight
			existing.reorgedAt = nil
		}
		return nil
	}
	s.deposits[k] = &fakeDepositState{Deposit: d}
	return nil
}

func (s *fakeTxStore2) ListNotEscrowed(_ context.Context) ([]Deposit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Deposit
	for _, ds := range s.deposits {
		if !ds.Escrowed && ds.reorgedAt == nil {
			out = append(out, ds.Deposit)
		}
	}
	return out, nil
}

func (s *fakeTxStore2) ListReadyToConfirm(_ context.Context, minDepth int, currentHeight int32) ([]Deposit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Deposit
	for _, ds := range s.deposits {
		if ds.Escrowed && !ds.confirmed && ds.reorgedAt == nil && int(currentHeight-ds.SeenAtHeight) >= minDepth {
			out = append(out, ds.Deposit)
		}
	}
	return out, nil
}

func (s *fakeTxStore2) ListReorgedEscrowed(_ context.Context, minAge time.Duration) ([]Deposit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	threshold := time.Now().Add(-minAge)
	var out []Deposit
	for _, ds := range s.deposits {
		if ds.reorgedAt != nil && ds.Escrowed && !ds.confirmed && ds.reorgedAt.Before(threshold) {
			out = append(out, ds.Deposit)
		}
	}
	return out, nil
}

func (s *fakeTxStore2) MarkEscrowed(_ context.Context, txid string, vout int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ds, ok := s.deposits[s.key(txid, vout)]; ok {
		ds.Escrowed = true
	}
	return nil
}

func (s *fakeTxStore2) MarkConfirmed(_ context.Context, txid string, vout int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ds, ok := s.deposits[s.key(txid, vout)]; ok {
		ds.confirmed = true
	}
	return nil
}

func (s *fakeTxStore2) MarkCancelled(_ context.Context, txid string, vout int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.deposits, s.key(txid, vout))
	return nil
}

func (s *fakeTxStore2) DeleteFromHeight(_ context.Context, height int32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, ds := range s.deposits {
		if ds.SeenAtHeight >= height && !ds.confirmed {
			if ds.Escrowed {
				ds.reorgedAt = &now // soft delete: cashier holds funds
			} else {
				delete(s.deposits, k) // hard delete: cashier was never notified
			}
		}
	}
	return nil
}

// fakeCashierClient records calls.
type fakeCashierClient struct {
	mu             sync.Mutex
	escrowedCalls  []cashierCall
	confirmCalls   []cashierCall
	cancelCalls    []cashierCall
	escrowErr      error
	confirmErr     error
	idempotencySet map[string]struct{}
}

type cashierCall struct {
	UserID     string
	Satoshis   int64
	DepositRef string
}

func newFakeCashierClient() *fakeCashierClient {
	return &fakeCashierClient{idempotencySet: make(map[string]struct{})}
}

func (c *fakeCashierClient) DepositEscrowed(_ context.Context, userID string, satoshis int64, depositRef string) error {
	if c.escrowErr != nil {
		return c.escrowErr
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := "esc:" + depositRef
	if _, seen := c.idempotencySet[key]; seen {
		return nil
	}
	c.idempotencySet[key] = struct{}{}
	c.escrowedCalls = append(c.escrowedCalls, cashierCall{userID, satoshis, depositRef})
	return nil
}

func (c *fakeCashierClient) ConfirmDeposit(_ context.Context, userID string, satoshis int64, depositRef string) error {
	if c.confirmErr != nil {
		return c.confirmErr
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := "conf:" + depositRef
	if _, seen := c.idempotencySet[key]; seen {
		return nil
	}
	c.idempotencySet[key] = struct{}{}
	c.confirmCalls = append(c.confirmCalls, cashierCall{userID, satoshis, depositRef})
	return nil
}

func (c *fakeCashierClient) CancelDeposit(_ context.Context, userID string, satoshis int64, depositRef string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := "cancel:" + depositRef
	if _, seen := c.idempotencySet[key]; seen {
		return nil
	}
	c.idempotencySet[key] = struct{}{}
	c.cancelCalls = append(c.cancelCalls, cashierCall{userID, satoshis, depositRef})
	return nil
}

// fakeUserStore holds a static list of user records.
type fakeUserStore struct {
	mu      sync.Mutex
	records []UserRecord
}

func newFakeUserStore(records ...UserRecord) *fakeUserStore {
	return &fakeUserStore{records: records}
}

func (s *fakeUserStore) ListAll(_ context.Context) ([]UserRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]UserRecord, len(s.records))
	copy(out, s.records)
	return out, nil
}

func (s *fakeUserStore) add(r UserRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, r)
}

// alwaysLeader is a leaderChecker stub that always reports leadership.
type alwaysLeader struct{}

func (alwaysLeader) IsLeader() bool { return true }

// Ensure fakes satisfy their interfaces at compile time.
var (
	_ SPVNode       = (*fakeSPVNode)(nil)
	_ TxStore       = (*fakeTxStore2)(nil)
	_ CashierClient = (*fakeCashierClient)(nil)
	_ UserStore     = (*fakeUserStore)(nil)
	_ leaderChecker = alwaysLeader{}
)

// Suppress unused import of wire.
var _ = (*wire.MsgTx)(nil)
