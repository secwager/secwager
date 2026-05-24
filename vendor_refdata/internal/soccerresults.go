package internal

import (
	"context"
	"fmt"
	"log"
	"strconv"

	pb "github.com/secwager/secwager/proto/gen/registry"
)

// apFixtureDetailResp is the response from /fixtures?id={fixtureId}.
type apFixtureDetailResp struct {
	Response []struct {
		Fixture struct {
			ID     int `json:"id"`
			Status struct {
				Short string `json:"short"`
			} `json:"status"`
		} `json:"fixture"`
		Goals struct {
			Home *int `json:"home"`
			Away *int `json:"away"`
		} `json:"goals"`
		Teams struct {
			Home struct {
				ID int `json:"id"`
			} `json:"home"`
			Away struct {
				ID int `json:"id"`
			} `json:"away"`
		} `json:"teams"`
	} `json:"response"`
}

// apFixturePlayersResp is the response from /fixtures/players?fixture={id}&team={id}.
// results=1 means one group (the team), players are nested inside.
type apFixturePlayersResp struct {
	Response []struct {
		Players []struct {
			Player struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"player"`
			Statistics []struct {
				Games struct {
					Position string `json:"position"`
				} `json:"games"`
				Goals struct {
					Total   *int `json:"total"`
					Assists *int `json:"assists"`
					Saves   *int `json:"saves"`
				} `json:"goals"`
				Cards struct {
					Yellow *int `json:"yellow"`
				} `json:"cards"`
			} `json:"statistics"`
		} `json:"players"`
	} `json:"response"`
}

// FetchSoccerResults checks active soccer games for completion and writes player stats.
func FetchSoccerResults(ctx context.Context, client *APISportsClient, store *Store, games []GameRecord) {
	for _, g := range games {
		if pb.League(g.LeagueID) != pb.League_EPL &&
			pb.League(g.LeagueID) != pb.League_LA_LIGA &&
			pb.League(g.LeagueID) != pb.League_MLS {
			continue
		}
		if err := fetchSoccerGameResult(ctx, client, store, g); err != nil {
			log.Printf("soccer results game %s: %v", g.ID, err)
		}
	}
}

func fetchSoccerGameResult(ctx context.Context, client *APISportsClient, store *Store, g GameRecord) error {
	var detail apFixtureDetailResp
	if err := client.GetSoccer(ctx, fmt.Sprintf("/fixtures?id=%s", g.ExternalID), &detail); err != nil {
		return fmt.Errorf("fixture detail: %w", err)
	}
	if len(detail.Response) == 0 {
		return nil
	}
	fix := detail.Response[0]
	if fix.Fixture.Status.Short != "FT" {
		return nil
	}

	homeScore := 0
	awayScore := 0
	if fix.Goals.Home != nil {
		homeScore = *fix.Goals.Home
	}
	if fix.Goals.Away != nil {
		awayScore = *fix.Goals.Away
	}

	// Fetch player stats for home and away teams.
	homeTeamExtID := strconv.Itoa(fix.Teams.Home.ID)
	awayTeamExtID := strconv.Itoa(fix.Teams.Away.ID)

	for _, teamExtID := range []string{homeTeamExtID, awayTeamExtID} {
		if err := fetchSoccerTeamStats(ctx, client, store, g.ID, g.ExternalID, teamExtID); err != nil {
			log.Printf("soccer team stats fixture=%s team=%s: %v", g.ExternalID, teamExtID, err)
		}
	}

	return store.MarkGameFinal(ctx, g.ID, homeScore, awayScore)
}

func fetchSoccerTeamStats(ctx context.Context, client *APISportsClient, store *Store, internalGameID, fixtureExtID, teamExtID string) error {
	var resp apFixturePlayersResp
	if err := client.GetSoccer(ctx,
		fmt.Sprintf("/fixtures/players?fixture=%s&team=%s", fixtureExtID, teamExtID),
		&resp); err != nil {
		return err
	}
	if len(resp.Response) == 0 {
		return nil
	}
	for _, p := range resp.Response[0].Players {
		if len(p.Statistics) == 0 {
			continue
		}
		playerExtID := strconv.Itoa(p.Player.ID)
		pr, ok, err := store.LookupPlayerByExternal(ctx, soccerVendor, playerExtID)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		stat := p.Statistics[0]
		coerceNil := func(v *int) int64 {
			if v == nil {
				return 0
			}
			return int64(*v)
		}
		stats := map[pb.PropType]int64{
			pb.PropType_GOALS:        coerceNil(stat.Goals.Total),
			pb.PropType_ASSISTS:      coerceNil(stat.Goals.Assists),
			pb.PropType_SAVES:        coerceNil(stat.Goals.Saves),
			pb.PropType_YELLOW_CARDS: coerceNil(stat.Cards.Yellow),
		}
		for pt, val := range stats {
			if err := store.UpsertPlayerStat(ctx, internalGameID, pr.ID, pt, val); err != nil {
				log.Printf("upsert soccer stat %s/%s/%v: %v", internalGameID, pr.ID, pt, err)
			}
		}
	}
	return nil
}
