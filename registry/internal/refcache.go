package internal

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pb "github.com/secwager/secwager/proto/gen/registry"
)

// RefCache maintains a short-lived in-memory snapshot of ref data refreshed
// from Postgres on a configurable interval. Prop rules are always sourced from
// the hardcoded defaultPropRules() — they are a product definition, not DB data.
type RefCache struct {
	pool     *pgxpool.Pool
	mu       sync.RWMutex
	snapshot *refStore
}

func newRefCache(pool *pgxpool.Pool) *RefCache {
	return &RefCache{pool: pool}
}

// Start performs the first load synchronously (returning once the initial
// snapshot is ready), then launches a background goroutine that refreshes
// on interval until ctx is cancelled.
func (c *RefCache) Start(ctx context.Context, interval time.Duration) {
	c.refresh(ctx)
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.refresh(ctx)
			}
		}
	}()
}

func (c *RefCache) refresh(ctx context.Context) {
	rs, err := loadRefFromDB(ctx, c.pool)
	if err != nil {
		log.Printf("refcache: refresh failed: %v", err)
		return
	}
	rs.propRules = defaultPropRules()
	c.mu.Lock()
	c.snapshot = rs
	c.mu.Unlock()
}

// get returns the current snapshot. Callers must not retain the pointer across
// a request boundary — the snapshot may be swapped at any time.
func (c *RefCache) get() *refStore {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.snapshot == nil {
		return &refStore{
			teams:      make(map[string]*pb.Team),
			players:    make(map[string]*pb.Player),
			games:      make(map[string]*pb.Game),
			gameRoster: make(map[string][]string),
			propRules:  defaultPropRules(),
		}
	}
	return c.snapshot
}
