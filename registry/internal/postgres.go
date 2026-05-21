package internal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pb "github.com/secwager/secwager/proto/gen/registry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type pgStore struct {
	pool *pgxpool.Pool
}

func newPGStore(pool *pgxpool.Pool) *pgStore {
	return &pgStore{pool: pool}
}

func (s *pgStore) Create(ctx context.Context, id string, expiry time.Time, legs []LegRow) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var existing string
	err = tx.QueryRow(ctx, `SELECT id FROM instruments WHERE id = $1`, id).Scan(&existing)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("check existing: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO instruments (id, expiry) VALUES ($1, $2)`,
		id, expiry,
	); err != nil {
		return false, fmt.Errorf("insert instrument: %w", err)
	}

	for _, leg := range legs {
		var legID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO instrument_legs
			  (instrument_id, leg_hash, game_id, league, outcome, player_id, prop_type, comparator, threshold, expiry_unix)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			RETURNING id`,
			id, leg.LegHash, leg.GameID, leg.League,
			leg.Outcome, leg.PlayerID, leg.PropType, leg.Comparator, leg.Threshold,
			leg.ExpiryUnix,
		).Scan(&legID)
		if err != nil {
			return false, fmt.Errorf("insert leg: %w", err)
		}

		for _, entityID := range leg.Entities {
			if _, err := tx.Exec(ctx,
				`INSERT INTO instrument_leg_entities (leg_id, instrument_id, entity_id) VALUES ($1,$2,$3)
				 ON CONFLICT DO NOTHING`,
				legID, id, entityID,
			); err != nil {
				return false, fmt.Errorf("insert entity: %w", err)
			}
		}
	}

	return false, tx.Commit(ctx)
}

func (s *pgStore) Get(ctx context.Context, id string) (InstrumentRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT l.game_id, l.league, l.outcome, l.player_id, l.prop_type, l.comparator, l.threshold,
		       i.expiry
		FROM instruments i
		JOIN instrument_legs l ON l.instrument_id = i.id
		WHERE i.id = $1`, id)
	if err != nil {
		return InstrumentRecord{}, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	rec := InstrumentRecord{ID: id}
	found := false
	for rows.Next() {
		found = true
		var (
			gameID    string
			league    int32
			outcome   *int32
			playerID  *string
			propType  *int32
			cmp       *int32
			threshold *int64
			expiry    time.Time
		)
		if err := rows.Scan(&gameID, &league, &outcome, &playerID, &propType, &cmp, &threshold, &expiry); err != nil {
			return InstrumentRecord{}, fmt.Errorf("scan: %w", err)
		}
		rec.ExpiryUnix = expiry.Unix()
		leg := &pb.Leg{GameId: gameID}
		if outcome != nil {
			leg.Predicate = &pb.Leg_GameOutcome{GameOutcome: &pb.GameOutcome{Outcome: pb.Outcome(*outcome)}}
		} else if playerID != nil && propType != nil && cmp != nil && threshold != nil {
			leg.Predicate = &pb.Leg_PlayerProp{PlayerProp: &pb.PlayerProp{
				PlayerId:   *playerID,
				PropType:   pb.PropType(*propType),
				Comparator: pb.Comparator(*cmp),
				Threshold:  *threshold,
			}}
		}
		rec.Legs = append(rec.Legs, leg)
	}
	if err := rows.Err(); err != nil {
		return InstrumentRecord{}, fmt.Errorf("rows: %w", err)
	}
	if !found {
		return InstrumentRecord{}, status.Errorf(codes.NotFound, "instrument %q not found", id)
	}
	return rec, nil
}

type pageToken struct {
	CreatedAt time.Time `json:"c"`
	ID        string    `json:"i"`
}

func encodePageToken(t time.Time, id string) string {
	b, _ := json.Marshal(pageToken{CreatedAt: t, ID: id})
	return base64.StdEncoding.EncodeToString(b)
}

func decodePageToken(s string) (pageToken, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return pageToken{}, err
	}
	var pt pageToken
	return pt, json.Unmarshal(b, &pt)
}

func (s *pgStore) List(ctx context.Context, req ListFilter) ([]InstrumentRecord, string, error) {
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}

	var pt pageToken
	if req.PageToken != "" {
		var err error
		pt, err = decodePageToken(req.PageToken)
		if err != nil {
			return nil, "", status.Error(codes.InvalidArgument, "invalid page_token")
		}
	}

	// Fetch one extra to detect next page.
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (i.created_at, i.id) i.id, i.expiry, i.created_at
		FROM instruments i
		JOIN instrument_legs l ON l.instrument_id = i.id
		JOIN instrument_leg_entities e ON e.leg_id = l.id
		WHERE ($1 = '' OR e.entity_id = $1)
		  AND ($2 = 0  OR l.league = $2)
		  AND (NOT $3  OR i.expiry > NOW())
		  AND ($4::timestamptz IS NULL OR (i.created_at, i.id) < ($4::timestamptz, $5))
		ORDER BY i.created_at DESC, i.id DESC
		LIMIT $6`,
		req.EntityID, req.League, req.ActiveOnly,
		nilIfZero(pt.CreatedAt), nilIfEmpty(pt.ID),
		pageSize+1,
	)
	if err != nil {
		return nil, "", fmt.Errorf("list query: %w", err)
	}
	defer rows.Close()

	type row struct {
		id        string
		expiry    time.Time
		createdAt time.Time
	}
	var rowList []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.expiry, &r.createdAt); err != nil {
			return nil, "", fmt.Errorf("scan: %w", err)
		}
		rowList = append(rowList, r)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("rows: %w", err)
	}

	var nextToken string
	if len(rowList) > pageSize {
		last := rowList[pageSize-1]
		nextToken = encodePageToken(last.createdAt, last.id)
		rowList = rowList[:pageSize]
	}

	out := make([]InstrumentRecord, 0, len(rowList))
	for _, r := range rowList {
		rec, err := s.Get(ctx, r.id)
		if err != nil {
			return nil, "", err
		}
		out = append(out, rec)
	}
	return out, nextToken, nil
}

func nilIfZero(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
