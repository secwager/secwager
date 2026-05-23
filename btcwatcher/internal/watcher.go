package internal

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
)

// Config holds Watcher runtime parameters.
type Config struct {
	Confirmations        int
	PollInterval         time.Duration
	CashierRetryInterval time.Duration // how often to retry pending cashier calls independent of block events
	ReorgCancelAge       time.Duration // min time after reorg before cancelling a dangling escrow; default 20m
	NetworkParams        *chaincfg.Params
	StartHeight          int32 // 0 = watch forward from current tip
}

// leaderChecker allows the Watcher to ask whether this node is the current
// leader without importing the etcd client. Tests inject a stub.
type leaderChecker interface {
	IsLeader() bool
}

// Watcher ties together the SPVNode, TxStore, UserStore, and CashierClient
// into a single deposit monitoring loop.
type Watcher struct {
	node    SPVNode
	txs     TxStore
	users   UserStore
	cashier CashierClient
	leader  leaderChecker
	cfg     Config
}

// NewWatcher constructs a Watcher. leader gates cashier calls: only the node
// holding the etcd lease will call DepositEscrowed / ConfirmDeposit. All nodes
// insert deposits and handle reorgs regardless.
func NewWatcher(node SPVNode, txs TxStore, users UserStore, cashier CashierClient, leader leaderChecker, cfg Config) *Watcher {
	return &Watcher{node: node, txs: txs, users: users, cashier: cashier, leader: leader, cfg: cfg}
}

// Run starts the Watcher and blocks until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) error {
	addrIndex, err := w.loadUsers(ctx)
	if err != nil {
		return fmt.Errorf("initial user load: %w", err)
	}

	pollTicker := time.NewTicker(w.cfg.PollInterval)
	defer pollTicker.Stop()

	cashierInterval := w.cfg.CashierRetryInterval
	if cashierInterval == 0 {
		cashierInterval = 30 * time.Second
	}
	cashierTicker := time.NewTicker(cashierInterval)
	defer cashierTicker.Stop()

	blockCh, err := w.node.Blocks(ctx, w.cfg.StartHeight)
	if err != nil {
		return fmt.Errorf("start block stream: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-pollTicker.C:
			if err := w.pollNewUsers(ctx, addrIndex); err != nil {
				// Non-fatal: log and continue.
				_ = err
			}

		case <-cashierTicker.C:
			if w.leader.IsLeader() {
				w.retryEscrows(ctx)
				w.cancelReorgedEscrows(ctx)
				if height, err := w.node.BestBlock(ctx); err == nil {
					w.checkConfirmations(ctx, height)
				}
			}

		case evt, ok := <-blockCh:
			if !ok {
				return nil
			}
			if evt.Connected {
				w.handleBlockConnected(ctx, evt, addrIndex)
			} else {
				_ = w.txs.DeleteFromHeight(ctx, evt.Height)
			}
		}
	}
}

// loadUsers fetches all users, registers their addresses with the SPV node,
// and returns a map of addrString → userID.
func (w *Watcher) loadUsers(ctx context.Context) (map[string]string, error) {
	records, err := w.users.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	addrIndex := make(map[string]string, len(records))
	var addrs []btcutil.Address
	for _, r := range records {
		addr, err := btcutil.DecodeAddress(r.BtcAddr, w.cfg.NetworkParams)
		if err != nil {
			continue
		}
		addrIndex[r.BtcAddr] = r.UserID
		addrs = append(addrs, addr)
	}
	if len(addrs) > 0 {
		_ = w.node.WatchAddresses(addrs)
	}
	return addrIndex, nil
}

// pollNewUsers diffs the current user list against addrIndex and registers
// any new addresses with the SPV node.
func (w *Watcher) pollNewUsers(ctx context.Context, addrIndex map[string]string) error {
	records, err := w.users.ListAll(ctx)
	if err != nil {
		return err
	}
	var newAddrs []btcutil.Address
	for _, r := range records {
		if _, seen := addrIndex[r.BtcAddr]; seen {
			continue
		}
		addr, err := btcutil.DecodeAddress(r.BtcAddr, w.cfg.NetworkParams)
		if err != nil {
			continue
		}
		addrIndex[r.BtcAddr] = r.UserID
		newAddrs = append(newAddrs, addr)
	}
	if len(newAddrs) > 0 {
		return w.node.WatchAddresses(newAddrs)
	}
	return nil
}

func (w *Watcher) handleBlockConnected(ctx context.Context, evt BlockEvent, addrIndex map[string]string) {
	for _, tx := range evt.RelevantTxs {
		txid := tx.TxHash().String()
		for vout, out := range tx.TxOut {
			addrStr, err := outputAddress(out.PkScript, w.cfg.NetworkParams)
			if err != nil {
				continue
			}
			userID, ok := addrIndex[addrStr]
			if !ok {
				continue
			}
			_ = w.txs.Insert(ctx, Deposit{
				Txid:         txid,
				Vout:         vout,
				UserID:       userID,
				Satoshis:     out.Value,
				SeenAtHeight: evt.Height,
			})
		}
	}
	if w.leader.IsLeader() {
		w.retryEscrows(ctx)
		w.checkConfirmations(ctx, evt.Height)
	}
}

func (w *Watcher) retryEscrows(ctx context.Context) {
	rows, err := w.txs.ListNotEscrowed(ctx)
	if err != nil {
		return
	}
	for _, d := range rows {
		ref := depositRef(d.Txid, d.Vout)
		if err := w.cashier.DepositEscrowed(ctx, d.UserID, d.Satoshis, ref); err != nil {
			continue
		}
		_ = w.txs.MarkEscrowed(ctx, d.Txid, d.Vout)
	}
}

func (w *Watcher) checkConfirmations(ctx context.Context, currentHeight int32) {
	rows, err := w.txs.ListReadyToConfirm(ctx, w.cfg.Confirmations, currentHeight)
	if err != nil {
		return
	}
	for _, d := range rows {
		ref := depositRef(d.Txid, d.Vout)
		if err := w.cashier.ConfirmDeposit(ctx, d.UserID, d.Satoshis, ref); err != nil {
			continue
		}
		_ = w.txs.MarkConfirmed(ctx, d.Txid, d.Vout)
	}
}

func (w *Watcher) cancelReorgedEscrows(ctx context.Context) {
	age := w.cfg.ReorgCancelAge
	if age == 0 {
		age = 20 * time.Minute
	}
	rows, err := w.txs.ListReorgedEscrowed(ctx, age)
	if err != nil {
		return
	}
	for _, d := range rows {
		ref := depositRef(d.Txid, d.Vout)
		if err := w.cashier.CancelDeposit(ctx, d.UserID, d.Satoshis, ref); err != nil {
			continue
		}
		_ = w.txs.MarkCancelled(ctx, d.Txid, d.Vout)
	}
}

func depositRef(txid string, vout int) string {
	return txid + ":" + strconv.Itoa(vout)
}
