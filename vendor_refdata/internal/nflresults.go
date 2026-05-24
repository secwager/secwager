package internal

import (
	"context"
	"fmt"
	"log"
	"strconv"

	pb "github.com/secwager/secwager/proto/gen/registry"
)

// apNFLGameDetailResp is the response from /games?id={gameId}.
type apNFLGameDetailResp struct {
	Response []struct {
		Game struct {
			Status struct {
				Long  string `json:"long"`
				Short string `json:"short"`
			} `json:"status"`
		} `json:"game"`
		Scores struct {
			Home struct {
				Total *int `json:"total"`
			} `json:"home"`
			Away struct {
				Total *int `json:"total"`
			} `json:"away"`
		} `json:"scores"`
	} `json:"response"`
}

// apNFLGameStatsResp is the response from /games/statistics/players?id={gameId}.
// The structure is: response[] (one per team) → groups[] → players[] → statistics[].
// Stat values are strings; some non-numeric (e.g. "11/15", "0-0") must be ignored.
type apNFLGameStatsResp struct {
	Response []struct {
		Team struct {
			ID int `json:"id"`
		} `json:"team"`
		Groups []struct {
			Name    string `json:"name"`
			Players []struct {
				Player struct {
					ID   int    `json:"id"`
					Name string `json:"name"`
				} `json:"player"`
				Statistics []struct {
					Name  string      `json:"name"`
					Value interface{} `json:"value"`
				} `json:"statistics"`
			} `json:"players"`
		} `json:"groups"`
	} `json:"response"`
}

// nflPlayerAccum accumulates stats for a single player across groups.
type nflPlayerAccum struct {
	passingYards   int64
	passingTDs     int64
	interceptions  int64
	rushingYards   int64
	rushingTDs     int64
	receivingYards int64
	receivingTDs   int64
	isQB           bool
}

// FetchNFLResults checks active NFL games for completion and writes player stats.
func FetchNFLResults(ctx context.Context, client *APISportsClient, store *Store, games []GameRecord) {
	for _, g := range games {
		if pb.League(g.LeagueID) != pb.League_NFL {
			continue
		}
		if err := fetchNFLGameResult(ctx, client, store, g); err != nil {
			log.Printf("nfl results game %s: %v", g.ID, err)
		}
	}
}

func fetchNFLGameResult(ctx context.Context, client *APISportsClient, store *Store, g GameRecord) error {
	var detail apNFLGameDetailResp
	if err := client.GetNFL(ctx, fmt.Sprintf("/games?id=%s", g.ExternalID), &detail); err != nil {
		return fmt.Errorf("nfl game detail: %w", err)
	}
	if len(detail.Response) == 0 {
		return nil
	}
	d := detail.Response[0]
	if d.Game.Status.Short != "FT" {
		return nil
	}

	homeScore := 0
	awayScore := 0
	if d.Scores.Home.Total != nil {
		homeScore = *d.Scores.Home.Total
	}
	if d.Scores.Away.Total != nil {
		awayScore = *d.Scores.Away.Total
	}

	if err := fetchNFLPlayerStats(ctx, client, store, g.ID, g.ExternalID); err != nil {
		log.Printf("nfl player stats game %s: %v", g.ID, err)
	}
	return store.MarkGameFinal(ctx, g.ID, homeScore, awayScore)
}

func fetchNFLPlayerStats(ctx context.Context, client *APISportsClient, store *Store, internalGameID, extGameID string) error {
	var resp apNFLGameStatsResp
	if err := client.GetNFL(ctx, fmt.Sprintf("/games/statistics/players?id=%s", extGameID), &resp); err != nil {
		return err
	}

	// First pass: accumulate stats per player ID across all groups in both teams.
	accum := make(map[int]*nflPlayerAccum)
	getAccum := func(id int) *nflPlayerAccum {
		if _, ok := accum[id]; !ok {
			accum[id] = &nflPlayerAccum{}
		}
		return accum[id]
	}

	for _, teamEntry := range resp.Response {
		for _, grp := range teamEntry.Groups {
			for _, p := range grp.Players {
				a := getAccum(p.Player.ID)
				sm := statMap(p.Statistics)
				switch grp.Name {
				case "Passing":
					a.isQB = true
					a.passingYards += statInt(sm, "yards")
					a.passingTDs += statInt(sm, "passing touch downs")
					a.interceptions += statInt(sm, "interceptions")
				case "Rushing":
					a.rushingYards += statInt(sm, "yards")
					a.rushingTDs += statInt(sm, "rushing touch downs")
				case "Receiving":
					a.receivingYards += statInt(sm, "yards")
					a.receivingTDs += statInt(sm, "receiving touch downs")
				}
			}
		}
	}

	// Second pass: look up internal player ID and write stats.
	for extID, a := range accum {
		pr, ok, err := store.LookupPlayerByExternal(ctx, nflVendor, strconv.Itoa(extID))
		if err != nil {
			return err
		}
		if !ok {
			continue
		}

		toWrite := map[pb.PropType]int64{}
		if a.isQB {
			toWrite[pb.PropType_PASSING_YARDS] = a.passingYards
			toWrite[pb.PropType_TOUCHDOWNS] = a.passingTDs
			toWrite[pb.PropType_INTERCEPTIONS] = a.interceptions
		} else {
			toWrite[pb.PropType_TOUCHDOWNS] = a.rushingTDs + a.receivingTDs
		}
		if a.rushingYards > 0 {
			toWrite[pb.PropType_RUSHING_YARDS] = a.rushingYards
		}
		if a.receivingYards > 0 {
			toWrite[pb.PropType_RECEIVING_YARDS] = a.receivingYards
		}

		for pt, val := range toWrite {
			if err := store.UpsertPlayerStat(ctx, internalGameID, pr.ID, pt, val); err != nil {
				log.Printf("upsert nfl stat %v: %v", pt, err)
			}
		}
	}
	return nil
}

// statMap builds a name→value map from a statistics array.
func statMap(stats []struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
}) map[string]interface{} {
	m := make(map[string]interface{}, len(stats))
	for _, s := range stats {
		m[s.Name] = s.Value
	}
	return m
}

// statInt extracts an integer value from a stat map. Returns 0 on any conversion failure.
func statInt(m map[string]interface{}, name string) int64 {
	v, ok := m[name]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int:
		return int64(val)
	case string:
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			// Try float (e.g. "11.1")
			f, err2 := strconv.ParseFloat(val, 64)
			if err2 != nil {
				return 0
			}
			return int64(f)
		}
		return n
	}
	return 0
}
