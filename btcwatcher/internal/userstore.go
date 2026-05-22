package internal

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRecord holds the fields btcwatcher needs from the userregistration DB.
type UserRecord struct {
	UserID  string
	BtcAddr string // P2WPKH bech32 address, pre-derived at registration time
}

// UserStore fetches users from the userregistration Postgres database (read-only).
type UserStore interface {
	ListAll(ctx context.Context) ([]UserRecord, error)
}

type postgresUserStore struct {
	pool *pgxpool.Pool
}

// NewPostgresUserStore returns a UserStore backed by the userregistration DB.
func NewPostgresUserStore(pool *pgxpool.Pool) UserStore {
	return &postgresUserStore{pool: pool}
}

func (s *postgresUserStore) ListAll(ctx context.Context) ([]UserRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT user_id, btc_addr FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []UserRecord
	for rows.Next() {
		var r UserRecord
		if err := rows.Scan(&r.UserID, &r.BtcAddr); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}
