package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	pb "github.com/secwager/secwager/proto/gen/registry"
)

func loadRefFromDB(ctx context.Context, pool *pgxpool.Pool) (*refStore, error) {
	rs := &refStore{
		teams:            make(map[string]*pb.Team),
		players:          make(map[string]*pb.Player),
		games:            make(map[string]*pb.Game),
		gameRoster:       make(map[string][]string),
		confirmedLineups: make(map[string]map[string]bool),
		propRules:        make(map[ruleKey][]pb.PropType),
	}

	rows, err := pool.Query(ctx, `SELECT id, name, short_name, league_id FROM teams`)
	if err != nil {
		return nil, fmt.Errorf("query teams: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, shortName string
		var leagueID int32
		if err := rows.Scan(&id, &name, &shortName, &leagueID); err != nil {
			return nil, fmt.Errorf("scan team: %w", err)
		}
		rs.teams[id] = &pb.Team{Id: id, Name: name, ShortName: shortName, League: pb.League(leagueID)}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("teams rows: %w", err)
	}

	prows, err := pool.Query(ctx, `SELECT id, name, team_id, positions FROM players`)
	if err != nil {
		return nil, fmt.Errorf("query players: %w", err)
	}
	defer prows.Close()
	for prows.Next() {
		var id, name, teamID string
		var positions []int32
		if err := prows.Scan(&id, &name, &teamID, &positions); err != nil {
			return nil, fmt.Errorf("scan player: %w", err)
		}
		pbPos := make([]pb.Position, len(positions))
		for i, p := range positions {
			pbPos[i] = pb.Position(p)
		}
		rs.players[id] = &pb.Player{Id: id, Name: name, TeamId: teamID, Positions: pbPos}
	}
	if err := prows.Err(); err != nil {
		return nil, fmt.Errorf("players rows: %w", err)
	}

	grows, err := pool.Query(ctx,
		`SELECT id, league_id, home_team_id, away_team_id, scheduled_unix, expiry_unix FROM games`)
	if err != nil {
		return nil, fmt.Errorf("query games: %w", err)
	}
	defer grows.Close()
	for grows.Next() {
		var id, homeID, awayID string
		var leagueID int32
		var scheduledUnix, expiryUnix int64
		if err := grows.Scan(&id, &leagueID, &homeID, &awayID, &scheduledUnix, &expiryUnix); err != nil {
			return nil, fmt.Errorf("scan game: %w", err)
		}
		rs.games[id] = &pb.Game{
			Id:            id,
			League:        pb.League(leagueID),
			HomeTeamId:    homeID,
			AwayTeamId:    awayID,
			ScheduledUnix: scheduledUnix,
			ExpiryUnix:    expiryUnix,
		}
	}
	if err := grows.Err(); err != nil {
		return nil, fmt.Errorf("games rows: %w", err)
	}

	for _, p := range rs.players {
		for _, g := range rs.games {
			if p.TeamId == g.HomeTeamId || p.TeamId == g.AwayTeamId {
				rs.gameRoster[g.Id] = append(rs.gameRoster[g.Id], p.Id)
			}
		}
	}

	lrows, err := pool.Query(ctx, `
		SELECT game_id, player_id FROM game_lineups
		WHERE game_id IN (
			SELECT id FROM games
			WHERE status != 'FINAL'
			  AND expiry_unix >= extract(epoch from now())::bigint
		)`)
	if err != nil {
		return nil, fmt.Errorf("query game_lineups: %w", err)
	}
	defer lrows.Close()
	for lrows.Next() {
		var gameID, playerID string
		if err := lrows.Scan(&gameID, &playerID); err != nil {
			return nil, fmt.Errorf("scan lineup: %w", err)
		}
		if rs.confirmedLineups[gameID] == nil {
			rs.confirmedLineups[gameID] = make(map[string]bool)
		}
		rs.confirmedLineups[gameID][playerID] = true
	}
	if err := lrows.Err(); err != nil {
		return nil, fmt.Errorf("lineup rows: %w", err)
	}

	return rs, nil
}
