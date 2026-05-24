package internal

import (
	"context"
	"log"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

const leaderElectionKey = "/secwager/btcwatcher/leader"

// LeaderElector campaigns for leadership via etcd and exposes IsLeader for
// the Watcher to gate cashier calls. Run in its own goroutine; never blocks
// block processing.
type LeaderElector struct {
	client *clientv3.Client
	nodeID string
	ttl    int
	mu     sync.RWMutex
	leading bool
}

// NewLeaderElector creates a LeaderElector. Session creation is deferred to
// Run so that a temporarily unavailable etcd does not prevent startup.
func NewLeaderElector(client *clientv3.Client, nodeID string, ttl int) *LeaderElector {
	return &LeaderElector{client: client, nodeID: nodeID, ttl: ttl}
}

// Run campaigns for leadership, retrying on transient etcd errors until ctx is
// cancelled. Meant to run in its own goroutine — does not block block
// processing in the Watcher. On becoming leader sets IsLeader() = true; resigns
// and closes the session on ctx cancellation.
func (e *LeaderElector) Run(ctx context.Context) {
	for {
		session, err := concurrency.NewSession(e.client, concurrency.WithTTL(e.ttl), concurrency.WithContext(ctx))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("leader election: session create failed, retrying in 5s: %v", err)
			select {
			case <-time.After(5 * time.Second):
				continue
			case <-ctx.Done():
				return
			}
		}

		election := concurrency.NewElection(session, leaderElectionKey)
		if err := election.Campaign(ctx, e.nodeID); err != nil {
			_ = session.Close()
			if ctx.Err() != nil {
				return
			}
			log.Printf("leader election: campaign failed, retrying in 5s: %v", err)
			select {
			case <-time.After(5 * time.Second):
				continue
			case <-ctx.Done():
				return
			}
		}

		e.mu.Lock()
		e.leading = true
		e.mu.Unlock()
		log.Printf("leader election: %s became leader", e.nodeID)

		<-ctx.Done()

		e.mu.Lock()
		e.leading = false
		e.mu.Unlock()

		resignCtx := context.Background()
		_ = election.Resign(resignCtx)
		_ = session.Close()
		return
	}
}

// IsLeader returns true if this node currently holds the leadership lease.
func (e *LeaderElector) IsLeader() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.leading
}
