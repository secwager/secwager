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

// FetchMLBRefData fetches teams, rosters, and schedule for the given season year,
// upserting everything into the store.
func FetchMLBRefData(ctx context.Context, client *MLBClient, store *Store, year int) error {
	if err := store.UpsertSeason(ctx, int(pb.League_MLB), year); err != nil {
		return fmt.Errorf("upsert MLB season: %w", err)
	}

	var teamsResp mlbTeamsResp
	if err := client.get(ctx, fmt.Sprintf("/teams?sportId=1&season=%d", year), &teamsResp); err != nil {
		return fmt.Errorf("fetch MLB teams: %w", err)
	}

	// Map numeric team ID → abbreviation for schedule processing.
	teamAbbr := make(map[int]string, len(teamsResp.Teams))
	for _, t := range teamsResp.Teams {
		teamAbbr[t.ID] = t.Abbreviation
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

	// Fetch rosters for every team.
	for _, t := range teamsResp.Teams {
		if err := fetchMLBRoster(ctx, client, store, t.ID, t.Abbreviation, year); err != nil {
			log.Printf("fetch MLB roster for %s: %v", t.Abbreviation, err)
		}
	}

	// Fetch schedule.
	var schedResp mlbScheduleResp
	if err := client.get(ctx, fmt.Sprintf("/schedule?sportId=1&season=%d&gameType=R", year), &schedResp); err != nil {
		return fmt.Errorf("fetch MLB schedule: %w", err)
	}
	for _, d := range schedResp.Dates {
		for _, g := range d.Games {
			homeAbbr := teamAbbr[g.Teams.Home.Team.ID]
			awayAbbr := teamAbbr[g.Teams.Away.Team.ID]
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
