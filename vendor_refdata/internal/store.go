package internal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	pb "github.com/secwager/secwager/proto/gen/registry"
)

type TeamRecord struct {
	ID             string
	Name           string
	ShortName      string
	LeagueID       int32
	ExternalID     string
	ExternalVendor string
}

type PlayerRecord struct {
	ID             string
	Name           string
	TeamID         string
	Positions      []int32
	ExternalID     string
	ExternalVendor string
}

type GameRecord struct {
	ID             string
	LeagueID       int32
	SeasonYear     int
	HomeTeamID     string
	AwayTeamID     string
	ScheduledUnix  int64
	ExpiryUnix     int64
	Status         string
	HomeScore      *int
	AwayScore      *int
	ExternalID     string
	ExternalVendor string
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) UpsertSeason(ctx context.Context, leagueID, year int) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO seasons (league_id, year) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		leagueID, year)
	return err
}

func (s *Store) UpsertTeam(ctx context.Context, t TeamRecord) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO teams (id, name, short_name, league_id, external_id, external_vendor, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (external_vendor, external_id) DO UPDATE SET
			name       = EXCLUDED.name,
			short_name = EXCLUDED.short_name,
			updated_at = now()`,
		t.ID, t.Name, t.ShortName, t.LeagueID, t.ExternalID, t.ExternalVendor)
	return err
}

// UpsertPlayer inserts or updates a player. On first insert, if the generated internal
// ID collides with an existing player (same last name on same team), a numeric suffix is
// appended (_2, _3, …).
func (s *Store) UpsertPlayer(ctx context.Context, p PlayerRecord) error {
	// Check if this external player already exists so we reuse its internal ID.
	var existingID string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM players WHERE external_vendor=$1 AND external_id=$2`,
		p.ExternalVendor, p.ExternalID).Scan(&existingID)
	if err == nil {
		_, err = s.pool.Exec(ctx,
			`UPDATE players SET name=$1, team_id=$2, positions=$3, updated_at=now() WHERE id=$4`,
			p.Name, p.TeamID, p.Positions, existingID)
		return err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("lookup player %s: %w", p.ExternalID, err)
	}

	// New player — insert with collision-safe ID.
	for suffix := 0; suffix <= 10; suffix++ {
		id := p.ID
		if suffix > 0 {
			id = fmt.Sprintf("%s_%d", p.ID, suffix+1)
		}
		_, err := s.pool.Exec(ctx, `
			INSERT INTO players (id, name, team_id, positions, external_id, external_vendor, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, now())`,
			id, p.Name, p.TeamID, p.Positions, p.ExternalID, p.ExternalVendor)
		if err == nil {
			return nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "pkey") {
			continue
		}
		return err
	}
	return fmt.Errorf("too many ID collisions for player %s", p.ID)
}

func (s *Store) UpsertGame(ctx context.Context, g GameRecord) error {
	// Step 1: if this external game was rescheduled (same gamePk, different date → different
	// internal id), update the existing row's id so the second step doesn't hit the
	// external_vendor+external_id unique constraint.
	_, err := s.pool.Exec(ctx, `
		UPDATE games
		SET id = $1, scheduled_unix = $2, expiry_unix = $3, updated_at = now()
		WHERE external_vendor = $4 AND external_id = $5 AND id != $1`,
		g.ID, g.ScheduledUnix, g.ExpiryUnix, g.ExternalVendor, g.ExternalID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "pkey") {
			// Target ID already exists (another game occupies the same slot).
			// Delete the stale row for this external game so step 2 can upsert on the target id.
			_, err = s.pool.Exec(ctx,
				`DELETE FROM games WHERE external_vendor = $1 AND external_id = $2 AND id != $3`,
				g.ExternalVendor, g.ExternalID, g.ID)
			if err != nil {
				return fmt.Errorf("delete stale rescheduled game: %w", err)
			}
		} else {
			return fmt.Errorf("rescheduled-game update: %w", err)
		}
	}

	// Step 2: insert or update by internal id.
	_, err = s.pool.Exec(ctx, `
		INSERT INTO games (id, league_id, season_year, home_team_id, away_team_id,
		                   scheduled_unix, expiry_unix, status, external_id, external_vendor, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
		ON CONFLICT (id) DO UPDATE SET
			scheduled_unix  = EXCLUDED.scheduled_unix,
			expiry_unix     = EXCLUDED.expiry_unix,
			external_id     = EXCLUDED.external_id,
			external_vendor = EXCLUDED.external_vendor,
			updated_at      = now()`,
		g.ID, g.LeagueID, g.SeasonYear, g.HomeTeamID, g.AwayTeamID,
		g.ScheduledUnix, g.ExpiryUnix, g.Status, g.ExternalID, g.ExternalVendor)
	return err
}

func (s *Store) UpsertPlayerStat(ctx context.Context, gameID, playerID string, propType pb.PropType, value int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO game_player_stats (game_id, player_id, prop_type, value, recorded_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (game_id, player_id, prop_type) DO UPDATE SET
			value       = EXCLUDED.value,
			recorded_at = now()`,
		gameID, playerID, int32(propType), value)
	return err
}

func (s *Store) LookupTeamByExternal(ctx context.Context, vendor, externalID string) (TeamRecord, bool, error) {
	var t TeamRecord
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, short_name, league_id, external_id, external_vendor
		 FROM teams WHERE external_vendor=$1 AND external_id=$2`,
		vendor, externalID).
		Scan(&t.ID, &t.Name, &t.ShortName, &t.LeagueID, &t.ExternalID, &t.ExternalVendor)
	if errors.Is(err, pgx.ErrNoRows) {
		return TeamRecord{}, false, nil
	}
	return t, err == nil, err
}

func (s *Store) LookupPlayerByExternal(ctx context.Context, vendor, externalID string) (PlayerRecord, bool, error) {
	var p PlayerRecord
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, team_id, positions, external_id, external_vendor
		 FROM players WHERE external_vendor=$1 AND external_id=$2`,
		vendor, externalID).
		Scan(&p.ID, &p.Name, &p.TeamID, &p.Positions, &p.ExternalID, &p.ExternalVendor)
	if errors.Is(err, pgx.ErrNoRows) {
		return PlayerRecord{}, false, nil
	}
	return p, err == nil, err
}

func (s *Store) LookupGameByExternal(ctx context.Context, vendor, externalID string) (GameRecord, bool, error) {
	var g GameRecord
	err := s.pool.QueryRow(ctx,
		`SELECT id, league_id, season_year, home_team_id, away_team_id,
		        scheduled_unix, expiry_unix, status, external_id, external_vendor
		 FROM games WHERE external_vendor=$1 AND external_id=$2`,
		vendor, externalID).
		Scan(&g.ID, &g.LeagueID, &g.SeasonYear, &g.HomeTeamID, &g.AwayTeamID,
			&g.ScheduledUnix, &g.ExpiryUnix, &g.Status, &g.ExternalID, &g.ExternalVendor)
	if errors.Is(err, pgx.ErrNoRows) {
		return GameRecord{}, false, nil
	}
	return g, err == nil, err
}

func (s *Store) LookupGame(ctx context.Context, internalID string) (GameRecord, bool, error) {
	var g GameRecord
	err := s.pool.QueryRow(ctx,
		`SELECT id, league_id, season_year, home_team_id, away_team_id,
		        scheduled_unix, expiry_unix, status, external_id, external_vendor
		 FROM games WHERE id=$1`, internalID).
		Scan(&g.ID, &g.LeagueID, &g.SeasonYear, &g.HomeTeamID, &g.AwayTeamID,
			&g.ScheduledUnix, &g.ExpiryUnix, &g.Status, &g.ExternalID, &g.ExternalVendor)
	if errors.Is(err, pgx.ErrNoRows) {
		return GameRecord{}, false, nil
	}
	return g, err == nil, err
}

// ListActiveGames returns games not yet FINAL that are in-window:
// started within the last 6 hours and not yet past their expiry.
func (s *Store) ListActiveGames(ctx context.Context) ([]GameRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, league_id, season_year, home_team_id, away_team_id,
		       scheduled_unix, expiry_unix, status, external_id, external_vendor
		FROM games
		WHERE status != 'FINAL'
		  AND scheduled_unix <= extract(epoch from now())::bigint + 1800
		  AND expiry_unix    >= extract(epoch from now())::bigint`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GameRecord
	for rows.Next() {
		var g GameRecord
		if err := rows.Scan(&g.ID, &g.LeagueID, &g.SeasonYear, &g.HomeTeamID, &g.AwayTeamID,
			&g.ScheduledUnix, &g.ExpiryUnix, &g.Status, &g.ExternalID, &g.ExternalVendor); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) MarkGameFinal(ctx context.Context, gameID string, homeScore, awayScore int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE games SET status='FINAL', home_score=$1, away_score=$2, updated_at=now() WHERE id=$3`,
		homeScore, awayScore, gameID)
	return err
}

// LookupTeamCodesByLeague returns externalID → short_name for a league+vendor.
// Used by schedule fetchers to resolve team codes without re-calling the vendor teams endpoint.
func (s *Store) LookupTeamCodesByLeague(ctx context.Context, leagueID int32, vendor string) (map[string]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT external_id, short_name FROM teams WHERE league_id=$1 AND external_vendor=$2`,
		leagueID, vendor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var extID, shortName string
		if err := rows.Scan(&extID, &shortName); err != nil {
			return nil, err
		}
		out[extID] = shortName
	}
	return out, rows.Err()
}

// ShouldFetch returns true if dataType has never been fetched or was last fetched more than interval ago.
func (s *Store) ShouldFetch(ctx context.Context, dataType string, interval time.Duration) (bool, error) {
	var fetchedAt time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT fetched_at FROM refdata_fetch_log WHERE data_type=$1`, dataType).Scan(&fetchedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return time.Since(fetchedAt) >= interval, nil
}

// RecordFetch upserts now() as the last-fetched time for dataType.
func (s *Store) RecordFetch(ctx context.Context, dataType string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO refdata_fetch_log (data_type, fetched_at) VALUES ($1, now())
		 ON CONFLICT (data_type) DO UPDATE SET fetched_at = now()`, dataType)
	return err
}

// ReplaceGameLineup atomically replaces all confirmed lineup entries for a game.
func (s *Store) ReplaceGameLineup(ctx context.Context, gameID string, playerIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM game_lineups WHERE game_id=$1`, gameID); err != nil {
		return err
	}
	for _, pid := range playerIDs {
		if _, err = tx.Exec(ctx,
			`INSERT INTO game_lineups (game_id, player_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			gameID, pid); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// HasLineup returns true if at least one confirmed lineup entry exists for the game.
func (s *Store) HasLineup(ctx context.Context, gameID string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM game_lineups WHERE game_id=$1`, gameID).Scan(&count)
	return count > 0, err
}

// ListUpcomingGames returns non-FINAL games whose kickoff is within [now-window, now+window].
func (s *Store) ListUpcomingGames(ctx context.Context, window time.Duration) ([]GameRecord, error) {
	windowSecs := int64(window.Seconds())
	rows, err := s.pool.Query(ctx, `
		SELECT id, league_id, season_year, home_team_id, away_team_id,
		       scheduled_unix, expiry_unix, status, external_id, external_vendor
		FROM games
		WHERE status != 'FINAL'
		  AND scheduled_unix BETWEEN extract(epoch from now())::bigint - $1
		                         AND extract(epoch from now())::bigint + $1`,
		windowSecs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GameRecord
	for rows.Next() {
		var g GameRecord
		if err := rows.Scan(&g.ID, &g.LeagueID, &g.SeasonYear, &g.HomeTeamID, &g.AwayTeamID,
			&g.ScheduledUnix, &g.ExpiryUnix, &g.Status, &g.ExternalID, &g.ExternalVendor); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
