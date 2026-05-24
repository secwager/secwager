package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	soccerBaseURL = "https://v3.football.api-sports.io"
	nflBaseURL    = "https://v1.american-football.api-sports.io"
)

// APISportsClient is shared by soccer and NFL callers. It enforces a 30s inter-request
// delay to stay well inside the free-tier rate limit (10 req/min).
type APISportsClient struct {
	key        string
	client     *http.Client
	mu         sync.Mutex
	last       time.Time
	throttle   time.Duration
	soccerBase string
	nflBase    string
}

func NewAPISportsClient(key string) *APISportsClient {
	return &APISportsClient{
		key:        key,
		client:     &http.Client{},
		throttle:   30 * time.Second,
		soccerBase: soccerBaseURL,
		nflBase:    nflBaseURL,
	}
}

// newTestAPISportsClient returns a client with rate limiting disabled (for tests).
func newTestAPISportsClient(key string) *APISportsClient {
	c := NewAPISportsClient(key)
	c.throttle = 0
	return c
}

func (c *APISportsClient) get(ctx context.Context, base, path string, out interface{}) error {
	if c.throttle > 0 {
		c.mu.Lock()
		if elapsed := time.Since(c.last); elapsed < c.throttle {
			wait := c.throttle - elapsed
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			c.mu.Lock()
		}
		c.last = time.Now()
		c.mu.Unlock()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-apisports-key", c.key)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("api-sports %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *APISportsClient) GetSoccer(ctx context.Context, path string, out interface{}) error {
	return c.get(ctx, c.soccerBase, path, out)
}

func (c *APISportsClient) GetNFL(ctx context.Context, path string, out interface{}) error {
	return c.get(ctx, c.nflBase, path, out)
}
