package internal

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Deposit represents a BTC transaction output sent to a watched address.
type Deposit struct {
	Txid          string
	Vout          int
	UserID        string
	Satoshis      int64
	SeenAtHeight  int32
	Escrowed      bool
}

// TxStore manages the btc_deposits table.
type TxStore interface {
	// Insert records a newly-seen output. No-op if (txid, vout) already exists.
	Insert(ctx context.Context, d Deposit) error
	// ListNotEscrowed returns rows where escrowed = FALSE (cashier not yet notified).
	ListNotEscrowed(ctx context.Context) ([]Deposit, error)
	// ListReadyToConfirm returns escrowed=TRUE, confirmed=FALSE rows that have
	// reached the required confirmation depth.
	ListReadyToConfirm(ctx context.Context, minDepth int, currentHeight int32) ([]Deposit, error)
	// MarkEscrowed sets escrowed=TRUE for the given (txid, vout).
	MarkEscrowed(ctx context.Context, txid string, vout int) error
	// MarkConfirmed sets confirmed=TRUE for the given (txid, vout).
	MarkConfirmed(ctx context.Context, txid string, vout int) error
	// DeleteFromHeight removes unconfirmed rows at or above height (re-org handling).
	DeleteFromHeight(ctx context.Context, height int32) error
}

type postgresTxStore struct {
	pool *pgxpool.Pool
}

// NewPostgresTxStore returns a TxStore backed by the btcwatcher Postgres DB.
func NewPostgresTxStore(pool *pgxpool.Pool) TxStore {
	return &postgresTxStore{pool: pool}
}

func (s *postgresTxStore) Insert(ctx context.Context, d Deposit) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO btc_deposits (txid, vout, user_id, satoshis, seen_at_height)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (txid, vout) DO NOTHING`,
		d.Txid, d.Vout, d.UserID, d.Satoshis, d.SeenAtHeight)
	return err
}

func (s *postgresTxStore) ListNotEscrowed(ctx context.Context) ([]Deposit, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT txid, vout, user_id, satoshis, seen_at_height
		FROM btc_deposits
		WHERE escrowed = FALSE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDeposits(rows)
}

func (s *postgresTxStore) ListReadyToConfirm(ctx context.Context, minDepth int, currentHeight int32) ([]Deposit, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT txid, vout, user_id, satoshis, seen_at_height
		FROM btc_deposits
		WHERE escrowed = TRUE AND confirmed = FALSE
		  AND $1 - seen_at_height >= $2`,
		currentHeight, minDepth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDeposits(rows)
}

func (s *postgresTxStore) MarkEscrowed(ctx context.Context, txid string, vout int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE btc_deposits SET escrowed = TRUE WHERE txid = $1 AND vout = $2`,
		txid, vout)
	return err
}

func (s *postgresTxStore) MarkConfirmed(ctx context.Context, txid string, vout int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE btc_deposits SET confirmed = TRUE WHERE txid = $1 AND vout = $2`,
		txid, vout)
	return err
}

func (s *postgresTxStore) DeleteFromHeight(ctx context.Context, height int32) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM btc_deposits WHERE seen_at_height >= $1 AND confirmed = FALSE`,
		height)
	return err
}

func scanDeposits(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]Deposit, error) {
	var deposits []Deposit
	for rows.Next() {
		var d Deposit
		if err := rows.Scan(&d.Txid, &d.Vout, &d.UserID, &d.Satoshis, &d.SeenAtHeight); err != nil {
			return nil, err
		}
		deposits = append(deposits, d)
	}
	return deposits, rows.Err()
}
