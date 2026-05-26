package internal

import (
	"context"
	"fmt"
	"log"
	"strconv"

	pb "github.com/secwager/secwager/proto/gen/registry"
)

const mlbVendor = "mlb_stats"

// mlbTeamsResp is the structure returned by /teams?sportId=1&season={year}.
type mlbTeamsResp struct {
	Teams []struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		Abbreviation string `json:"abbreviation"`
	} `json:"teams"`
}

// mlbRosterResp is the structure returned by /teams/{id}/roster.
type mlbRosterResp struct {
	Roster []struct {
		Person struct {
			ID       int    `json:"id"`
			FullName string `json:"fullName"`
		} `json:"person"`
		Position struct {
			Type string `json:"type"`
		} `json:"position"`
	} `json:"roster"`
}

// mlbScheduleResp is used for both the full schedule and the status-check call.
type mlbScheduleResp struct {
	Dates []struct {
		Date  string `json:"date"`
		Games []struct {
			GamePk     int    `json:"gamePk"`
			GameDate   string `json:"gameDate"`
			GameNumber int    `json:"gameNumber"`
			Status     struct {
				DetailedState string `json:"detailedState"`
			} `json:"status"`
			Teams struct {
				Home struct {
					Score int `json:"score"`
					Team  struct {
						ID int `json:"id"`
					} `json:"team"`
				} `json:"home"`
				Away struct {
					Score int `json:"score"`
					Team  struct {
						ID int `json:"id"`
					} `json:"team"`
				} `json:"away"`
			} `json:"teams"`
		} `json:"games"`
	} `json:"dates"`
}

// FetchMLBRosters fetches MLB teams and per-team rosters for the given season year.
func FetchMLBRosters(ctx context.Context, client *MLBClient, store *Store, year int) error {
	if err := store.UpsertSeason(ctx, int(pb.League_MLB), year); err != nil {
		return fmt.Errorf("upsert MLB season: %w", err)
	}

	var teamsResp mlbTeamsResp
	if err := client.get(ctx, fmt.Sprintf("/teams?sportId=1&season=%d", year), &teamsResp); err != nil {
		return fmt.Errorf("fetch MLB teams: %w", err)
	}
	for _, t := range teamsResp.Teams {
		rec := TeamRecord{
			ID:             makeTeamID(pb.League_MLB, t.Abbreviation),
			Name:           t.Name,
			ShortName:      t.Abbreviation,
			LeagueID:       int32(pb.League_MLB),
			ExternalID:     strconv.Itoa(t.ID),
			ExternalVendor: mlbVendor,
		}
		if err := store.UpsertTeam(ctx, rec); err != nil {
			log.Printf("upsert MLB team %s: %v", t.Abbreviation, err)
		}
	}
	for _, t := range teamsResp.Teams {
		if err := fetchMLBRoster(ctx, client, store, t.ID, t.Abbreviation, year); err != nil {
			log.Printf("fetch MLB roster for %s: %v", t.Abbreviation, err)
		}
	}
	return nil
}

// FetchMLBSchedule fetches the MLB regular-season schedule, resolving team codes from the DB.
func FetchMLBSchedule(ctx context.Context, client *MLBClient, store *Store, year int) error {
	teamCodes, err := store.LookupTeamCodesByLeague(ctx, int32(pb.League_MLB), mlbVendor)
	if err != nil {
		return fmt.Errorf("lookup MLB team codes: %w", err)
	}

	var schedResp mlbScheduleResp
	if err := client.get(ctx, fmt.Sprintf("/schedule?sportId=1&season=%d&gameType=R", year), &schedResp); err != nil {
		return fmt.Errorf("fetch MLB schedule: %w", err)
	}
	for _, d := range schedResp.Dates {
		for _, g := range d.Games {
			homeAbbr := teamCodes[strconv.Itoa(g.Teams.Home.Team.ID)]
			awayAbbr := teamCodes[strconv.Itoa(g.Teams.Away.Team.ID)]
			if homeAbbr == "" || awayAbbr == "" {
				continue
			}
			ts, err := parseISO8601Unix(g.GameDate)
			if err != nil {
				log.Printf("parse game date %q: %v", g.GameDate, err)
				continue
			}
			rec := GameRecord{
				ID:             makeGameID(pb.League_MLB, awayAbbr, homeAbbr, ts, g.GameNumber),
				LeagueID:       int32(pb.League_MLB),
				SeasonYear:     year,
				HomeTeamID:     makeTeamID(pb.League_MLB, homeAbbr),
				AwayTeamID:     makeTeamID(pb.League_MLB, awayAbbr),
				ScheduledUnix:  ts,
				ExpiryUnix:     ts + 4*3600,
				Status:         "SCHEDULED",
				ExternalID:     strconv.Itoa(g.GamePk),
				ExternalVendor: mlbVendor,
			}
			if err := store.UpsertGame(ctx, rec); err != nil {
				log.Printf("upsert MLB game %d: %v", g.GamePk, err)
			}
		}
	}
	return nil
}

func fetchMLBRoster(ctx context.Context, client *MLBClient, store *Store, teamNumericID int, abbr string, year int) error {
	var resp mlbRosterResp
	if err := client.get(ctx, fmt.Sprintf("/teams/%d/roster?rosterType=active&season=%d", teamNumericID, year), &resp); err != nil {
		return err
	}
	teamInternalID := makeTeamID(pb.League_MLB, abbr)
	for _, r := range resp.Roster {
		pos, ok := mlbPositionMap[r.Position.Type]
		if !ok {
			continue
		}
		rec := PlayerRecord{
			ID:             makePlayerID(pb.League_MLB, abbr, r.Person.FullName),
			Name:           r.Person.FullName,
			TeamID:         teamInternalID,
			Positions:      []int32{int32(pos)},
			ExternalID:     strconv.Itoa(r.Person.ID),
			ExternalVendor: mlbVendor,
		}
		if err := store.UpsertPlayer(ctx, rec); err != nil {
			log.Printf("upsert MLB player %s: %v", r.Person.FullName, err)
		}
	}
	return nil
}

// FetchMLBRefData fetches rosters and schedule for the given season. Kept for test compatibility.
func FetchMLBRefData(ctx context.Context, client *MLBClient, store *Store, year int) error {
	if err := FetchMLBRosters(ctx, client, store, year); err != nil {
		return err
	}
	return FetchMLBSchedule(ctx, client, store, year)
}

// mlbLiveResp is the response from /api/v1.1/game/{gamePk}/feed/live.
// We only decode the fields needed to derive the starting lineup.
type mlbLiveResp struct {
	LiveData struct {
		Boxscore struct {
			Teams struct {
				Home struct {
					Batters []int `json:"batters"`
				} `json:"home"`
				Away struct {
					Batters []int `json:"batters"`
				} `json:"away"`
			} `json:"teams"`
		} `json:"boxscore"`
	} `json:"liveData"`
}

// FetchMLBLineup fetches confirmed batters from the MLB live feed and stores them as lineup entries.
// Returns (false, nil) if the batters array is empty (lineup not yet posted).
func FetchMLBLineup(ctx context.Context, client *MLBClient, store *Store, game GameRecord) (bool, error) {
	var resp mlbLiveResp
	if err := client.getV11(ctx, fmt.Sprintf("/game/%s/feed/live", game.ExternalID), &resp); err != nil {
		return false, fmt.Errorf("fetch MLB live feed game=%s: %w", game.ExternalID, err)
	}
	home := resp.LiveData.Boxscore.Teams.Home.Batters
	away := resp.LiveData.Boxscore.Teams.Away.Batters
	if len(home) == 0 && len(away) == 0 {
		return false, nil
	}

	var confirmedIDs []string
	for _, playerNumID := range append(home, away...) {
		p, ok, err := store.LookupPlayerByExternal(ctx, mlbVendor, strconv.Itoa(playerNumID))
		if err != nil {
			return false, fmt.Errorf("lookup MLB player %d: %w", playerNumID, err)
		}
		if ok {
			confirmedIDs = append(confirmedIDs, p.ID)
		}
	}
	if err := store.ReplaceGameLineup(ctx, game.ID, confirmedIDs); err != nil {
		return false, fmt.Errorf("replace lineup game=%s: %w", game.ID, err)
	}
	return true, nil
}

// parseISO8601Unix parses an ISO 8601 UTC timestamp (e.g. "2024-04-01T17:05:00Z") to Unix.
func parseISO8601Unix(s string) (int64, error) {
	// Try with fractional seconds too.
	formats := []string{"2006-01-02T15:04:05Z", "2006-01-02T15:04:05Z07:00"}
	for _, f := range formats {
		if t, err := parseTime(f, s); err == nil {
			return t, nil
		}
	}
	return 0, fmt.Errorf("cannot parse %q as ISO 8601", s)
}
