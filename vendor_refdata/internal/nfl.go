package internal

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	pb "github.com/secwager/secwager/proto/gen/registry"
)

const nflVendor = "apisports"

// apNFLTeamsResp is the response from /teams?league=1&season={year}.
// Note: team fields are at the top level of each response item (not nested under "team").
type apNFLTeamsResp struct {
	Response []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		Code string `json:"code"`
	} `json:"response"`
}

// apNFLPlayersResp is the response from /players?id={teamId}&season={year}.
type apNFLPlayersResp struct {
	Response []struct {
		Player struct {
			ID       int    `json:"id"`
			Name     string `json:"name"`
			Position string `json:"position"`
		} `json:"player"`
		Team struct {
			ID int `json:"id"`
		} `json:"team"`
	} `json:"response"`
}

// apNFLGamesResp is the response from /games?league=1&season={year}&week={n}.
type apNFLGamesResp struct {
	Response []struct {
		Game struct {
			ID     int `json:"id"`
			Status struct {
				Long  string `json:"long"`
				Short string `json:"short"`
			} `json:"status"`
			Stage    string `json:"stage"`
			Week     string `json:"week"`
			Date     struct {
				Timestamp int64 `json:"timestamp"`
			} `json:"date"`
		} `json:"game"`
		Teams struct {
			Home struct {
				ID int `json:"id"`
			} `json:"home"`
			Away struct {
				ID int `json:"id"`
			} `json:"away"`
		} `json:"teams"`
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

// FetchNFLRefData fetches NFL teams, players, and game schedule for all weeks.
func FetchNFLRefData(ctx context.Context, client *APISportsClient, store *Store, year int) error {
	if err := store.UpsertSeason(ctx, int(pb.League_NFL), year); err != nil {
		return fmt.Errorf("upsert NFL season: %w", err)
	}

	var teamsResp apNFLTeamsResp
	if err := client.GetNFL(ctx, fmt.Sprintf("/teams?league=1&season=%d", year), &teamsResp); err != nil {
		return fmt.Errorf("fetch NFL teams: %w", err)
	}

	teamCodeByID := make(map[int]string, len(teamsResp.Response))
	for _, t := range teamsResp.Response {
		teamCodeByID[t.ID] = t.Code
		rec := TeamRecord{
			ID:             makeTeamID(pb.League_NFL, t.Code),
			Name:           t.Name,
			ShortName:      t.Code,
			LeagueID:       int32(pb.League_NFL),
			ExternalID:     strconv.Itoa(t.ID),
			ExternalVendor: nflVendor,
		}
		if err := store.UpsertTeam(ctx, rec); err != nil {
			log.Printf("upsert NFL team %s: %v", t.Code, err)
		}
	}

	// Rosters: one call per team.
	for _, t := range teamsResp.Response {
		if err := fetchNFLRoster(ctx, client, store, t.ID, t.Code, year); err != nil {
			log.Printf("fetch NFL roster team %s: %v", t.Code, err)
		}
	}

	// Schedule: NFL has 18 regular-season weeks plus playoffs.
	// Fetch weeks 1–22 (regular season + postseason); skip empty weeks gracefully.
	for week := 1; week <= 22; week++ {
		if err := fetchNFLWeek(ctx, client, store, year, week, teamCodeByID); err != nil {
			log.Printf("fetch NFL week %d: %v", week, err)
		}
	}
	return nil
}

func fetchNFLRoster(ctx context.Context, client *APISportsClient, store *Store, teamNumericID int, code string, year int) error {
	var resp apNFLPlayersResp
	if err := client.GetNFL(ctx, fmt.Sprintf("/players?id=%d&season=%d", teamNumericID, year), &resp); err != nil {
		return err
	}
	teamInternalID := makeTeamID(pb.League_NFL, code)
	for _, item := range resp.Response {
		pos, ok := nflPositionMap[item.Player.Position]
		if !ok {
			continue
		}
		rec := PlayerRecord{
			ID:             makePlayerID(pb.League_NFL, code, item.Player.Name),
			Name:           item.Player.Name,
			TeamID:         teamInternalID,
			Positions:      []int32{int32(pos)},
			ExternalID:     strconv.Itoa(item.Player.ID),
			ExternalVendor: nflVendor,
		}
		if err := store.UpsertPlayer(ctx, rec); err != nil {
			log.Printf("upsert NFL player %s: %v", item.Player.Name, err)
		}
	}
	return nil
}

func fetchNFLWeek(ctx context.Context, client *APISportsClient, store *Store, year, week int, teamCodeByID map[int]string) error {
	var resp apNFLGamesResp
	if err := client.GetNFL(ctx, fmt.Sprintf("/games?league=1&season=%d&week=%d", year, week), &resp); err != nil {
		return err
	}
	for _, item := range resp.Response {
		homeCode := teamCodeByID[item.Teams.Home.ID]
		awayCode := teamCodeByID[item.Teams.Away.ID]
		if homeCode == "" || awayCode == "" {
			continue
		}
		ts := item.Game.Date.Timestamp
		if ts == 0 {
			continue
		}
		_ = time.Unix(ts, 0) // validate
		rec := GameRecord{
			ID:             makeGameID(pb.League_NFL, awayCode, homeCode, ts, 1),
			LeagueID:       int32(pb.League_NFL),
			SeasonYear:     year,
			HomeTeamID:     makeTeamID(pb.League_NFL, homeCode),
			AwayTeamID:     makeTeamID(pb.League_NFL, awayCode),
			ScheduledUnix:  ts,
			ExpiryUnix:     ts + 4*3600,
			Status:         "SCHEDULED",
			ExternalID:     strconv.Itoa(item.Game.ID),
			ExternalVendor: nflVendor,
		}
		if err := store.UpsertGame(ctx, rec); err != nil {
			log.Printf("upsert NFL game %d: %v", item.Game.ID, err)
		}
	}
	return nil
}
