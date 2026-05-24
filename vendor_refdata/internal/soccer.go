package internal

import (
	"context"
	"fmt"
	"log"
	"strconv"

)

const soccerVendor = "apisports"

// apSoccerTeamsResp is the response from /teams?league={id}&season={year}.
type apSoccerTeamsResp struct {
	Response []struct {
		Team struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
			Code string `json:"code"`
		} `json:"team"`
	} `json:"response"`
}

// apSoccerPlayersResp is the response from /players?league={id}&season={year}&page={n}.
type apSoccerPlayersResp struct {
	Paging struct {
		Current int `json:"current"`
		Total   int `json:"total"`
	} `json:"paging"`
	Response []struct {
		Player struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"player"`
		Statistics []struct {
			Team struct {
				ID int `json:"id"`
			} `json:"team"`
			Games struct {
				Position string `json:"position"`
			} `json:"games"`
		} `json:"statistics"`
	} `json:"response"`
}

// apSoccerFixturesResp is the response from /fixtures?league={id}&season={year}.
type apSoccerFixturesResp struct {
	Response []struct {
		Fixture struct {
			ID        int    `json:"id"`
			Timestamp int64  `json:"timestamp"`
			Status    struct {
				Short string `json:"short"`
			} `json:"status"`
		} `json:"fixture"`
		Teams struct {
			Home struct {
				ID int `json:"id"`
			} `json:"home"`
			Away struct {
				ID int `json:"id"`
			} `json:"away"`
		} `json:"teams"`
		Goals struct {
			Home *int `json:"home"`
			Away *int `json:"away"`
		} `json:"goals"`
	} `json:"response"`
}

// FetchSoccerRefData fetches teams, players, and fixture schedule for the given league and season.
func FetchSoccerRefData(ctx context.Context, client *APISportsClient, store *Store, leagueID int, year int) error {
	league, ok := soccerLeagueMap[leagueID]
	if !ok {
		return fmt.Errorf("unknown soccer league ID %d", leagueID)
	}
	if err := store.UpsertSeason(ctx, int(league), year); err != nil {
		return fmt.Errorf("upsert soccer season: %w", err)
	}

	// Teams.
	var teamsResp apSoccerTeamsResp
	if err := client.GetSoccer(ctx, fmt.Sprintf("/teams?league=%d&season=%d", leagueID, year), &teamsResp); err != nil {
		return fmt.Errorf("fetch soccer teams league=%d: %w", leagueID, err)
	}
	teamCodeByID := make(map[int]string, len(teamsResp.Response))
	for _, item := range teamsResp.Response {
		t := item.Team
		teamCodeByID[t.ID] = t.Code
		rec := TeamRecord{
			ID:             makeTeamID(league, t.Code),
			Name:           t.Name,
			ShortName:      t.Code,
			LeagueID:       int32(league),
			ExternalID:     strconv.Itoa(t.ID),
			ExternalVendor: soccerVendor,
		}
		if err := store.UpsertTeam(ctx, rec); err != nil {
			log.Printf("upsert soccer team %s: %v", t.Code, err)
		}
	}

	// Players — paginate through all pages.
	for page := 1; ; page++ {
		var resp apSoccerPlayersResp
		if err := client.GetSoccer(ctx,
			fmt.Sprintf("/players?league=%d&season=%d&page=%d", leagueID, year, page),
			&resp); err != nil {
			return fmt.Errorf("fetch soccer players page %d: %w", page, err)
		}
		for _, item := range resp.Response {
			if len(item.Statistics) == 0 {
				continue
			}
			stat := item.Statistics[0]
			pos, ok := soccerPlayerListPositionMap[stat.Games.Position]
			if !ok {
				continue
			}
			teamCode := teamCodeByID[stat.Team.ID]
			if teamCode == "" {
				continue
			}
			rec := PlayerRecord{
				ID:             makePlayerID(league, teamCode, item.Player.Name),
				Name:           item.Player.Name,
				TeamID:         makeTeamID(league, teamCode),
				Positions:      []int32{int32(pos)},
				ExternalID:     strconv.Itoa(item.Player.ID),
				ExternalVendor: soccerVendor,
			}
			if err := store.UpsertPlayer(ctx, rec); err != nil {
				log.Printf("upsert soccer player %s: %v", item.Player.Name, err)
			}
		}
		if resp.Paging.Current >= resp.Paging.Total {
			break
		}
	}

	// Fixtures.
	var fixturesResp apSoccerFixturesResp
	if err := client.GetSoccer(ctx, fmt.Sprintf("/fixtures?league=%d&season=%d", leagueID, year), &fixturesResp); err != nil {
		return fmt.Errorf("fetch soccer fixtures league=%d: %w", leagueID, err)
	}
	for _, item := range fixturesResp.Response {
		homeCode := teamCodeByID[item.Teams.Home.ID]
		awayCode := teamCodeByID[item.Teams.Away.ID]
		if homeCode == "" || awayCode == "" {
			continue
		}
		ts := item.Fixture.Timestamp
		expiryOffset := int64(2 * 3600) // 2h for soccer
		rec := GameRecord{
			ID:             makeGameID(league, awayCode, homeCode, ts, 1),
			LeagueID:       int32(league),
			SeasonYear:     year,
			HomeTeamID:     makeTeamID(league, homeCode),
			AwayTeamID:     makeTeamID(league, awayCode),
			ScheduledUnix:  ts,
			ExpiryUnix:     ts + expiryOffset,
			Status:         "SCHEDULED",
			ExternalID:     strconv.Itoa(item.Fixture.ID),
			ExternalVendor: soccerVendor,
		}
		if err := store.UpsertGame(ctx, rec); err != nil {
			log.Printf("upsert soccer game %d: %v", item.Fixture.ID, err)
		}
	}
	return nil
}

// soccerLeagueIDs lists all leagues managed by the soccer fetcher.
var soccerLeagueIDs = []int{39, 140, 253}
