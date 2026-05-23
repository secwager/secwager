package internal

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/walletdb"
	_ "github.com/btcsuite/btcwallet/walletdb/bdb"
	"github.com/lightninglabs/neutrino"
	"github.com/lightninglabs/neutrino/headerfs"
)

// BlockEvent is delivered for each block connected to or disconnected from the chain.
type BlockEvent struct {
	Height      int32
	Connected   bool           // true = BlockConnected, false = BlockDisconnected
	RelevantTxs []*wire.MsgTx // non-nil only on BlockConnected; filtered to watched addresses
}

// SPVNode is the interface btcwatcher uses to interact with the Bitcoin network.
type SPVNode interface {
	// WatchAddresses registers additional addresses to monitor.
	WatchAddresses(addrs []btcutil.Address) error
	// Blocks returns a channel that receives BlockEvents until ctx is cancelled.
	// startHeight 0 means watch forward from the current chain tip; >0 rescans from that height.
	Blocks(ctx context.Context, startHeight int32) (<-chan BlockEvent, error)
	// BestBlock returns the current synced chain tip height.
	BestBlock(ctx context.Context) (int32, error)
	// Stop shuts down the underlying node gracefully.
	Stop() error
}

type neutrinoSPVNode struct {
	cs     *neutrino.ChainService
	params *chaincfg.Params
	mu     sync.Mutex
	addrs  []btcutil.Address
	rescan *neutrino.Rescan
}

// NewNeutrinoSPVNode creates and starts a neutrino ChainService.
// dataDir is where neutrino persists its header and filter chain.
// peers is an optional list of Bitcoin P2P peers to connect to first.
func NewNeutrinoSPVNode(dataDir string, params *chaincfg.Params, peers []string) (SPVNode, error) {
	dbPath := filepath.Join(dataDir, "neutrino.db")
	db, err := walletdb.Create("bdb", dbPath, true, 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("open neutrino db: %w", err)
	}

	cfg := neutrino.Config{
		DataDir:     dataDir,
		Database:    db,
		ChainParams: *params,
		AddPeers:    peers,
	}
	cs, err := neutrino.NewChainService(cfg)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("new chain service: %w", err)
	}
	if err := cs.Start(); err != nil {
		return nil, fmt.Errorf("start chain service: %w", err)
	}
	return &neutrinoSPVNode{cs: cs, params: params}, nil
}

func (n *neutrinoSPVNode) WatchAddresses(addrs []btcutil.Address) error {
	n.mu.Lock()
	n.addrs = append(n.addrs, addrs...)
	r := n.rescan
	n.mu.Unlock()

	if r != nil {
		return r.Update(neutrino.AddAddrs(addrs...))
	}
	return nil
}

func (n *neutrinoSPVNode) Blocks(ctx context.Context, startHeight int32) (<-chan BlockEvent, error) {
	ch := make(chan BlockEvent, 16)
	quit := make(chan struct{})

	go func() {
		<-ctx.Done()
		close(quit)
	}()

	n.mu.Lock()
	watchAddrs := make([]btcutil.Address, len(n.addrs))
	copy(watchAddrs, n.addrs)
	n.mu.Unlock()

	blockCh := make(chan BlockEvent, 16)

	rescanOpts := []neutrino.RescanOption{}
	if startHeight > 0 {
		rescanOpts = append(rescanOpts, neutrino.StartBlock(&headerfs.BlockStamp{Height: startHeight}))
	}
	rescanOpts = append(rescanOpts,
		neutrino.QuitChan(quit),
		neutrino.WatchAddrs(watchAddrs...),
		neutrino.NotificationHandlers(rpcclient.NotificationHandlers{
			OnFilteredBlockConnected: func(height int32, header *wire.BlockHeader, txs []*btcutil.Tx) {
				msgTxs := make([]*wire.MsgTx, len(txs))
				for i, tx := range txs {
					msgTxs[i] = tx.MsgTx()
				}
				select {
				case blockCh <- BlockEvent{Height: height, Connected: true, RelevantTxs: msgTxs}:
				case <-quit:
				}
			},
			OnFilteredBlockDisconnected: func(height int32, header *wire.BlockHeader) {
				select {
				case blockCh <- BlockEvent{Height: height, Connected: false}:
				case <-quit:
				}
			},
		}),
	)

	r := neutrino.NewRescan(&neutrino.RescanChainSource{ChainService: n.cs}, rescanOpts...)

	n.mu.Lock()
	n.rescan = r
	n.mu.Unlock()

	go func() {
		defer close(ch)
		errChan := r.Start()

		for {
			select {
			case evt, ok := <-blockCh:
				if !ok {
					return
				}
				select {
				case ch <- evt:
				case <-quit:
					return
				}
			case err := <-errChan:
				if err != nil {
					_ = err
				}
				return
			case <-quit:
				return
			}
		}
	}()

	return ch, nil
}

func (n *neutrinoSPVNode) BestBlock(_ context.Context) (int32, error) {
	bs, err := n.cs.BestBlock()
	if err != nil {
		return 0, err
	}
	return bs.Height, nil
}

func (n *neutrinoSPVNode) Stop() error {
	return n.cs.Stop()
}
