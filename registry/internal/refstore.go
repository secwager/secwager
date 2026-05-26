package internal

import (
	"encoding/json"
	"fmt"
	"os"

	pb "github.com/secwager/secwager/proto/gen/registry"
)

type ruleKey struct {
	League   pb.League
	Position pb.Position
}

type refStore struct {
	teams            map[string]*pb.Team
	players          map[string]*pb.Player
	games            map[string]*pb.Game
	gameRoster       map[string][]string         // game_id → []player_id
	confirmedLineups map[string]map[string]bool  // game_id → set of player_ids
	propRules        map[ruleKey][]pb.PropType
}

type fixtureFile struct {
	Teams []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		ShortName string `json:"short_name"`
		League    string `json:"league"`
	} `json:"teams"`
	Players []struct {
		ID        string   `json:"id"`
		Name      string   `json:"name"`
		TeamID    string   `json:"team_id"`
		Positions []string `json:"positions"`
	} `json:"players"`
	Games []struct {
		ID           string `json:"id"`
		League       string `json:"league"`
		HomeTeamID   string `json:"home_team_id"`
		AwayTeamID   string `json:"away_team_id"`
		ScheduledUnix int64 `json:"scheduled_unix"`
		ExpiryUnix   int64 `json:"expiry_unix"`
	} `json:"games"`
	PropTypeRules []struct {
		League   string   `json:"league"`
		Position string   `json:"position"`
		Allowed  []string `json:"allowed"`
	} `json:"prop_type_rules"`
}

func loadFixture(path string) (*refStore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixture: %w", err)
	}
	var f fixtureFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse fixture: %w", err)
	}

	rs := &refStore{
		teams:            make(map[string]*pb.Team),
		players:          make(map[string]*pb.Player),
		games:            make(map[string]*pb.Game),
		gameRoster:       make(map[string][]string),
		confirmedLineups: make(map[string]map[string]bool),
		propRules:        make(map[ruleKey][]pb.PropType),
	}

	for _, t := range f.Teams {
		league, err := parseLeague(t.League)
		if err != nil {
			return nil, fmt.Errorf("team %s: %w", t.ID, err)
		}
		rs.teams[t.ID] = &pb.Team{Id: t.ID, Name: t.Name, ShortName: t.ShortName, League: league}
	}

	for _, p := range f.Players {
		var positions []pb.Position
		for _, pos := range p.Positions {
			position, err := parsePosition(pos)
			if err != nil {
				return nil, fmt.Errorf("player %s: %w", p.ID, err)
			}
			positions = append(positions, position)
		}
		rs.players[p.ID] = &pb.Player{Id: p.ID, Name: p.Name, TeamId: p.TeamID, Positions: positions}
	}

	for _, g := range f.Games {
		league, err := parseLeague(g.League)
		if err != nil {
			return nil, fmt.Errorf("game %s: %w", g.ID, err)
		}
		rs.games[g.ID] = &pb.Game{
			Id: g.ID, League: league,
			HomeTeamId: g.HomeTeamID, AwayTeamId: g.AwayTeamID,
			ScheduledUnix: g.ScheduledUnix, ExpiryUnix: g.ExpiryUnix,
		}
	}

	// Build roster: a player is in a game if their team is home or away.
	for _, p := range rs.players {
		for _, g := range rs.games {
			if p.TeamId == g.HomeTeamId || p.TeamId == g.AwayTeamId {
				rs.gameRoster[g.Id] = append(rs.gameRoster[g.Id], p.Id)
			}
		}
	}

	for _, rule := range f.PropTypeRules {
		league, err := parseLeague(rule.League)
		if err != nil {
			return nil, fmt.Errorf("prop rule: %w", err)
		}
		position, err := parsePosition(rule.Position)
		if err != nil {
			return nil, fmt.Errorf("prop rule: %w", err)
		}
		var props []pb.PropType
		for _, name := range rule.Allowed {
			pt, err := parsePropType(name)
			if err != nil {
				return nil, fmt.Errorf("prop rule %s/%s: %w", rule.League, rule.Position, err)
			}
			props = append(props, pt)
		}
		rs.propRules[ruleKey{League: league, Position: position}] = props
	}

	return rs, nil
}

func (rs *refStore) getTeam(id string) (*pb.Team, bool) {
	t, ok := rs.teams[id]
	return t, ok
}

func (rs *refStore) getGame(id string) (*pb.Game, bool) {
	g, ok := rs.games[id]
	return g, ok
}

func (rs *refStore) getPlayer(id string) (*pb.Player, bool) {
	p, ok := rs.players[id]
	return p, ok
}

func (rs *refStore) listTeams(league pb.League) []*pb.Team {
	var out []*pb.Team
	for _, t := range rs.teams {
		if league == pb.League_LEAGUE_UNSPECIFIED || t.League == league {
			out = append(out, t)
		}
	}
	return out
}

func (rs *refStore) listGames(league pb.League, fromUnix, toUnix int64) []*pb.Game {
	var out []*pb.Game
	for _, g := range rs.games {
		if league != pb.League_LEAGUE_UNSPECIFIED && g.League != league {
			continue
		}
		if fromUnix > 0 && g.ScheduledUnix < fromUnix {
			continue
		}
		if toUnix > 0 && g.ScheduledUnix > toUnix {
			continue
		}
		out = append(out, g)
	}
	return out
}

func (rs *refStore) listPlayersByGame(gameID string) []*pb.Player {
	ids := rs.gameRoster[gameID]
	confirmed := rs.confirmedLineups[gameID]
	out := make([]*pb.Player, 0, len(ids))
	for _, id := range ids {
		if p, ok := rs.players[id]; ok {
			if confirmed[id] {
				clone := *p
				clone.LineupConfirmed = true
				out = append(out, &clone)
			} else {
				out = append(out, p)
			}
		}
	}
	return out
}

func (rs *refStore) playerInGame(playerID, gameID string) bool {
	for _, id := range rs.gameRoster[gameID] {
		if id == playerID {
			return true
		}
	}
	return false
}

func (rs *refStore) allowedPropTypes(league pb.League, position pb.Position) []pb.PropType {
	return rs.propRules[ruleKey{League: league, Position: position}]
}

func parseLeague(s string) (pb.League, error) {
	v, ok := pb.League_value[s]
	if !ok || v == 0 {
		return 0, fmt.Errorf("unknown league %q", s)
	}
	return pb.League(v), nil
}

func parsePosition(s string) (pb.Position, error) {
	v, ok := pb.Position_value[s]
	if !ok || v == 0 {
		return 0, fmt.Errorf("unknown position %q", s)
	}
	return pb.Position(v), nil
}

func parsePropType(s string) (pb.PropType, error) {
	v, ok := pb.PropType_value[s]
	if !ok || v == 0 {
		return 0, fmt.Errorf("unknown prop type %q", s)
	}
	return pb.PropType(v), nil
}
