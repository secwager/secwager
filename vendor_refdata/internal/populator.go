package internal

import (
	"context"
	"log"
	"time"
)

// Populator runs the reference-data fetch cycle (teams, players, schedules).
type Populator struct {
	mlbClient *MLBClient
	apClient  *APISportsClient
	store     *Store
	year      int
}

func NewPopulator(mlbClient *MLBClient, apClient *APISportsClient, store *Store, year int) *Populator {
	return &Populator{mlbClient: mlbClient, apClient: apClient, store: store, year: year}
}

// RunOnce fetches reference data for all supported leagues.
func (p *Populator) RunOnce(ctx context.Context) error {
	log.Printf("populator: starting reference data fetch (year=%d)", p.year)
	if err := FetchMLBRefData(ctx, p.mlbClient, p.store, p.year); err != nil {
		log.Printf("populator: MLB ref data: %v", err)
	}
	for _, leagueID := range soccerLeagueIDs {
		if err := FetchSoccerRefData(ctx, p.apClient, p.store, leagueID, p.year); err != nil {
			log.Printf("populator: soccer league %d: %v", leagueID, err)
		}
	}
	if err := FetchNFLRefData(ctx, p.apClient, p.store, p.year); err != nil {
		log.Printf("populator: NFL ref data: %v", err)
	}
	log.Printf("populator: reference data fetch complete")
	return nil
}

// Run starts the reference data ticker. Blocks until ctx is cancelled.
func (p *Populator) Run(ctx context.Context, interval time.Duration) {
	p.RunOnce(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.RunOnce(ctx)
		}
	}
}
