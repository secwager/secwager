package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const mlbBaseURL = "https://statsapi.mlb.com/api/v1"

type MLBClient struct {
	base   string
	client *http.Client
}

func NewMLBClient() *MLBClient {
	return &MLBClient{base: mlbBaseURL, client: &http.Client{}}
}

func (c *MLBClient) get(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("MLB API %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
