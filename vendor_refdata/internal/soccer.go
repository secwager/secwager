package internal

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"
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
			ID        int   `json:"id"`
			Timestamp int64 `json:"timestamp"`
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

// apSoccerLineupsResp is the response from /fixtures/lineups?fixture={id}.
type apSoccerLineupsResp struct {
	Response []struct {
		StartXI []struct {
			Player struct {
				ID  int    `json:"id"`
				Pos string `json:"pos"`
			} `json:"player"`
		} `json:"startXI"`
	} `json:"response"`
}

// FetchSoccerRosters fetches teams and players for the given league and season.
func FetchSoccerRosters(ctx context.Context, client *APISportsClient, store *Store, leagueID int, year int) error {
	league, ok := soccerLeagueMap[leagueID]
	if !ok {
		return fmt.Errorf("unknown soccer league ID %d", leagueID)
	}
	year = soccerSeasonYear(leagueID, year)
	if err := store.UpsertSeason(ctx, int(league), year); err != nil {
		return fmt.Errorf("upsert soccer season: %w", err)
	}

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
	return nil
}

// FetchSoccerSchedule fetches fixtures for the given league, resolving team codes from the DB.
func FetchSoccerSchedule(ctx context.Context, client *APISportsClient, store *Store, leagueID int, year int) error {
	league, ok := soccerLeagueMap[leagueID]
	if !ok {
		return fmt.Errorf("unknown soccer league ID %d", leagueID)
	}
	year = soccerSeasonYear(leagueID, year)

	teamCodes, err := store.LookupTeamCodesByLeague(ctx, int32(league), soccerVendor)
	if err != nil {
		return fmt.Errorf("lookup soccer team codes league=%d: %w", leagueID, err)
	}

	var fixturesResp apSoccerFixturesResp
	if err := client.GetSoccer(ctx, fmt.Sprintf("/fixtures?league=%d&season=%d", leagueID, year), &fixturesResp); err != nil {
		return fmt.Errorf("fetch soccer fixtures league=%d: %w", leagueID, err)
	}
	for _, item := range fixturesResp.Response {
		homeCode := teamCodes[strconv.Itoa(item.Teams.Home.ID)]
		awayCode := teamCodes[strconv.Itoa(item.Teams.Away.ID)]
		if homeCode == "" || awayCode == "" {
			continue
		}
		ts := item.Fixture.Timestamp
		rec := GameRecord{
			ID:             makeGameID(league, awayCode, homeCode, ts, 1),
			LeagueID:       int32(league),
			SeasonYear:     year,
			HomeTeamID:     makeTeamID(league, homeCode),
			AwayTeamID:     makeTeamID(league, awayCode),
			ScheduledUnix:  ts,
			ExpiryUnix:     ts + 2*3600,
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

// FetchSoccerLineup fetches the confirmed starting XI for a single fixture.
// Returns (true, nil) if a lineup was found and stored; (false, nil) if not yet published.
func FetchSoccerLineup(ctx context.Context, client *APISportsClient, store *Store, game GameRecord) (bool, error) {
	var resp apSoccerLineupsResp
	if err := client.GetSoccer(ctx, fmt.Sprintf("/fixtures/lineups?fixture=%s", game.ExternalID), &resp); err != nil {
		return false, fmt.Errorf("fetch lineup fixture=%s: %w", game.ExternalID, err)
	}
	if len(resp.Response) == 0 {
		return false, nil
	}

	var confirmedIDs []string
	for _, teamEntry := range resp.Response {
		for _, xi := range teamEntry.StartXI {
			p, ok, err := store.LookupPlayerByExternal(ctx, soccerVendor, strconv.Itoa(xi.Player.ID))
			if err != nil {
				return false, fmt.Errorf("lookup player %d: %w", xi.Player.ID, err)
			}
			if ok {
				confirmedIDs = append(confirmedIDs, p.ID)
			}
		}
	}
	if err := store.ReplaceGameLineup(ctx, game.ID, confirmedIDs); err != nil {
		return false, fmt.Errorf("replace lineup game=%s: %w", game.ID, err)
	}
	return true, nil
}

// FetchSoccerRefData fetches rosters and schedule for one league. Kept for test compatibility.
func FetchSoccerRefData(ctx context.Context, client *APISportsClient, store *Store, leagueID int, year int) error {
	if err := FetchSoccerRosters(ctx, client, store, leagueID, year); err != nil {
		return err
	}
	return FetchSoccerSchedule(ctx, client, store, leagueID, year)
}

// soccerLeagueIDs lists all leagues managed by the soccer fetcher.
var soccerLeagueIDs = []int{39, 140, 253}

// europeanLeagues use a start-year season convention (season 2025 = 2025-26).
// Their season starts in August, so before August we need year-1 to get the current season.
var europeanLeagues = map[int]bool{39: true, 140: true}

func soccerSeasonYear(leagueID, year int) int {
	if europeanLeagues[leagueID] && time.Now().Month() < time.August {
		return year - 1
	}
	return year
}
