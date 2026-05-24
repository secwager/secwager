package internal

import (
	"context"
	"log"
	"time"

	pb "github.com/secwager/secwager/proto/gen/registry"
)

// EventFetcher polls active games for completion and writes player stats.
type EventFetcher struct {
	mlbClient *MLBClient
	apClient  *APISportsClient
	store     *Store
}

func NewEventFetcher(mlbClient *MLBClient, apClient *APISportsClient, store *Store) *EventFetcher {
	return &EventFetcher{mlbClient: mlbClient, apClient: apClient, store: store}
}

// RunOnce fetches results for all currently active games.
func (e *EventFetcher) RunOnce(ctx context.Context) error {
	games, err := e.store.ListActiveGames(ctx)
	if err != nil {
		return err
	}
	if len(games) == 0 {
		return nil
	}
	log.Printf("eventfetcher: checking %d active games", len(games))

	// Partition by sport.
	var mlb, soccer, nfl []GameRecord
	for _, g := range games {
		switch pb.League(g.LeagueID) {
		case pb.League_MLB:
			mlb = append(mlb, g)
		case pb.League_EPL, pb.League_LA_LIGA, pb.League_MLS:
			soccer = append(soccer, g)
		case pb.League_NFL:
			nfl = append(nfl, g)
		}
	}

	if len(mlb) > 0 {
		FetchMLBResults(ctx, e.mlbClient, e.store, mlb)
	}
	if len(soccer) > 0 {
		FetchSoccerResults(ctx, e.apClient, e.store, soccer)
	}
	if len(nfl) > 0 {
		FetchNFLResults(ctx, e.apClient, e.store, nfl)
	}
	return nil
}

// Run starts the results polling ticker. Blocks until ctx is cancelled.
func (e *EventFetcher) Run(ctx context.Context, interval time.Duration) {
	e.RunOnce(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.RunOnce(ctx)
		}
	}
}
