package internal

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	pb "github.com/secwager/secwager/proto/gen/registry"
)

// mlbBoxscoreResp is the response from /game/{gamePk}/boxscore.
type mlbBoxscoreResp struct {
	Teams struct {
		Home mlbBoxscoreTeam `json:"home"`
		Away mlbBoxscoreTeam `json:"away"`
	} `json:"teams"`
}

type mlbBoxscoreTeam struct {
	Players map[string]mlbBoxscorePlayer `json:"players"`
}

type mlbBoxscorePlayer struct {
	Person struct {
		ID int `json:"id"`
	} `json:"person"`
	Stats struct {
		Batting struct {
			HomeRuns    int `json:"homeRuns"`
			Hits        int `json:"hits"`
			Rbi         int `json:"rbi"`
			BaseOnBalls int `json:"baseOnBalls"`
		} `json:"batting"`
		Pitching struct {
			StrikeOuts  int `json:"strikeOuts"`
			EarnedRuns  int `json:"earnedRuns"`
			BaseOnBalls int `json:"baseOnBalls"`
		} `json:"pitching"`
	} `json:"stats"`
}

// FetchMLBResults checks active MLB games for completion and writes player stats.
func FetchMLBResults(ctx context.Context, client *MLBClient, store *Store, games []GameRecord) {
	// Collect gamePks for all active MLB games.
	var pks []string
	pkToGame := make(map[string]GameRecord)
	for _, g := range games {
		if pb.League(g.LeagueID) != pb.League_MLB {
			continue
		}
		pks = append(pks, g.ExternalID)
		pkToGame[g.ExternalID] = g
	}
	if len(pks) == 0 {
		return
	}

	// Batch status check.
	var schedResp mlbScheduleResp
	if err := client.get(ctx,
		"/schedule?gamePks="+strings.Join(pks, ","),
		&schedResp); err != nil {
		log.Printf("mlb schedule status check: %v", err)
		return
	}

	for _, d := range schedResp.Dates {
		for _, sg := range d.Games {
			if sg.Status.DetailedState != "Final" {
				continue
			}
			pk := strconv.Itoa(sg.GamePk)
			g, ok := pkToGame[pk]
			if !ok {
				continue
			}
			homeScore := sg.Teams.Home.Score
			awayScore := sg.Teams.Away.Score

			if err := fetchMLBBoxscore(ctx, client, store, g.ID, sg.GamePk); err != nil {
				log.Printf("mlb boxscore gamePk=%d: %v", sg.GamePk, err)
				continue
			}
			if err := store.MarkGameFinal(ctx, g.ID, homeScore, awayScore); err != nil {
				log.Printf("mlb mark final %s: %v", g.ID, err)
			}
		}
	}
}

func fetchMLBBoxscore(ctx context.Context, client *MLBClient, store *Store, internalGameID string, gamePk int) error {
	var resp mlbBoxscoreResp
	if err := client.get(ctx, fmt.Sprintf("/game/%d/boxscore", gamePk), &resp); err != nil {
		return err
	}

	for _, side := range []mlbBoxscoreTeam{resp.Teams.Home, resp.Teams.Away} {
		for _, p := range side.Players {
			extID := strconv.Itoa(p.Person.ID)
			pr, ok, err := store.LookupPlayerByExternal(ctx, mlbVendor, extID)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}

			hasBattingStats := p.Stats.Batting.HomeRuns > 0 || p.Stats.Batting.Hits > 0 ||
				p.Stats.Batting.Rbi > 0 || p.Stats.Batting.BaseOnBalls > 0
			hasPitchingStats := p.Stats.Pitching.StrikeOuts > 0 || p.Stats.Pitching.EarnedRuns > 0 ||
				p.Stats.Pitching.BaseOnBalls > 0

			if hasBattingStats {
				for pt, val := range map[pb.PropType]int64{
					pb.PropType_HOMERUNS: int64(p.Stats.Batting.HomeRuns),
					pb.PropType_HITS:     int64(p.Stats.Batting.Hits),
					pb.PropType_RBIS:     int64(p.Stats.Batting.Rbi),
					pb.PropType_WALKS:    int64(p.Stats.Batting.BaseOnBalls),
				} {
					if err := store.UpsertPlayerStat(ctx, internalGameID, pr.ID, pt, val); err != nil {
						log.Printf("upsert mlb batting stat %v: %v", pt, err)
					}
				}
			}
			if hasPitchingStats {
				for pt, val := range map[pb.PropType]int64{
					pb.PropType_STRIKEOUTS:  int64(p.Stats.Pitching.StrikeOuts),
					pb.PropType_EARNED_RUNS: int64(p.Stats.Pitching.EarnedRuns),
					pb.PropType_WALKS:       int64(p.Stats.Pitching.BaseOnBalls),
				} {
					if err := store.UpsertPlayerStat(ctx, internalGameID, pr.ID, pt, val); err != nil {
						log.Printf("upsert mlb pitching stat %v: %v", pt, err)
					}
				}
			}
		}
	}
	return nil
}
