package internal

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "github.com/secwager/secwager/proto/gen/registry"
)

// RosterPopulator fetches teams and player rosters on a daily cadence.
type RosterPopulator struct {
	mlbClient *MLBClient
	apClient  *APISportsClient
	store     *Store
	year      int
}

func NewRosterPopulator(mlbClient *MLBClient, apClient *APISportsClient, store *Store, year int) *RosterPopulator {
	return &RosterPopulator{mlbClient: mlbClient, apClient: apClient, store: store, year: year}
}

func (p *RosterPopulator) RunOnce(ctx context.Context, interval time.Duration) error {
	ok, err := p.store.ShouldFetch(ctx, "roster", interval)
	if err != nil {
		return err
	}
	if !ok {
		log.Printf("roster populator: skipping (fetched recently)")
		return nil
	}
	log.Printf("roster populator: starting (year=%d)", p.year)
	if err := FetchMLBRosters(ctx, p.mlbClient, p.store, p.year); err != nil {
		log.Printf("roster populator: MLB: %v", err)
	}
	for _, leagueID := range soccerLeagueIDs {
		if err := FetchSoccerRosters(ctx, p.apClient, p.store, leagueID, p.year); err != nil {
			log.Printf("roster populator: soccer league %d: %v", leagueID, err)
		}
	}
	if err := FetchNFLRosters(ctx, p.apClient, p.store, p.year); err != nil {
		log.Printf("roster populator: NFL: %v", err)
	}
	if err := p.store.RecordFetch(ctx, "roster"); err != nil {
		log.Printf("roster populator: record fetch: %v", err)
	}
	log.Printf("roster populator: done")
	return nil
}

func (p *RosterPopulator) Run(ctx context.Context, interval time.Duration) {
	p.RunOnce(ctx, interval)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.RunOnce(ctx, interval)
		}
	}
}

// SchedulePopulator fetches game schedules on a daily cadence.
type SchedulePopulator struct {
	mlbClient *MLBClient
	apClient  *APISportsClient
	store     *Store
	year      int
}

func NewSchedulePopulator(mlbClient *MLBClient, apClient *APISportsClient, store *Store, year int) *SchedulePopulator {
	return &SchedulePopulator{mlbClient: mlbClient, apClient: apClient, store: store, year: year}
}

func (p *SchedulePopulator) RunOnce(ctx context.Context, interval time.Duration) error {
	ok, err := p.store.ShouldFetch(ctx, "schedule", interval)
	if err != nil {
		return err
	}
	if !ok {
		log.Printf("schedule populator: skipping (fetched recently)")
		return nil
	}
	log.Printf("schedule populator: starting (year=%d)", p.year)
	if err := FetchMLBSchedule(ctx, p.mlbClient, p.store, p.year); err != nil {
		log.Printf("schedule populator: MLB: %v", err)
	}
	for _, leagueID := range soccerLeagueIDs {
		if err := FetchSoccerSchedule(ctx, p.apClient, p.store, leagueID, p.year); err != nil {
			log.Printf("schedule populator: soccer league %d: %v", leagueID, err)
		}
	}
	if err := FetchNFLSchedule(ctx, p.apClient, p.store, p.year); err != nil {
		log.Printf("schedule populator: NFL: %v", err)
	}
	if err := p.store.RecordFetch(ctx, "schedule"); err != nil {
		log.Printf("schedule populator: record fetch: %v", err)
	}
	log.Printf("schedule populator: done")
	return nil
}

func (p *SchedulePopulator) Run(ctx context.Context, interval time.Duration) {
	p.RunOnce(ctx, interval)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.RunOnce(ctx, interval)
		}
	}
}

// LineupPopulator polls upcoming games for confirmed starting lineups.
type LineupPopulator struct {
	mlbClient *MLBClient
	apClient  *APISportsClient
	store     *Store
	window    time.Duration
}

func NewLineupPopulator(mlbClient *MLBClient, apClient *APISportsClient, store *Store, window time.Duration) *LineupPopulator {
	return &LineupPopulator{mlbClient: mlbClient, apClient: apClient, store: store, window: window}
}

func (p *LineupPopulator) RunOnce(ctx context.Context) error {
	games, err := p.store.ListUpcomingGames(ctx, p.window)
	if err != nil {
		return fmt.Errorf("list upcoming games: %w", err)
	}
	for _, g := range games {
		hasLineup, err := p.store.HasLineup(ctx, g.ID)
		if err != nil {
			log.Printf("lineup populator: HasLineup %s: %v", g.ID, err)
			continue
		}
		if hasLineup {
			continue
		}
		switch pb.League(g.LeagueID) {
		case pb.League_MLB:
			if _, err := FetchMLBLineup(ctx, p.mlbClient, p.store, g); err != nil {
				log.Printf("lineup populator: MLB game %s: %v", g.ID, err)
			}
		case pb.League_EPL, pb.League_LA_LIGA, pb.League_MLS:
			if _, err := FetchSoccerLineup(ctx, p.apClient, p.store, g); err != nil {
				log.Printf("lineup populator: soccer game %s: %v", g.ID, err)
			}
		case pb.League_NFL:
			if _, err := FetchNFLLineup(ctx, p.apClient, p.store, g); err != nil {
				log.Printf("lineup populator: NFL game %s: %v", g.ID, err)
			}
		}
	}
	return nil
}

func (p *LineupPopulator) Run(ctx context.Context, interval time.Duration) {
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
