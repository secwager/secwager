package internal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	pb "github.com/secwager/secwager/cashier/gen/cashier"
)

type PostgresCashier struct {
	pool          *pgxpool.Pool
	idempotencyTTL time.Duration
}

func NewPostgresCashier(pool *pgxpool.Pool, idempotencyTTL time.Duration) *PostgresCashier {
	return &PostgresCashier{pool: pool, idempotencyTTL: idempotencyTTL}
}

func (c *PostgresCashier) Deposit(ctx context.Context, userID, idempotencyKey string, amount int64) (AccountSnapshot, error) {
	if amount <= 0 {
		return AccountSnapshot{}, fmt.Errorf("amount must be > 0")
	}
	if idempotencyKey == "" {
		return AccountSnapshot{}, fmt.Errorf("idempotency_key is required")
	}
	return c.withIdempotency(ctx, idempotencyKey, func(tx pgx.Tx) (AccountSnapshot, error) {
		_, err := tx.Exec(ctx, `
			INSERT INTO accounts (user_id, gross_balance, escrowed, version)
			VALUES ($1, $2, 0, 0)
			ON CONFLICT (user_id) DO UPDATE
				SET gross_balance = accounts.gross_balance + $2,
				    version       = accounts.version + 1,
				    updated_at    = clock_timestamp()`,
			userID, amount)
		if err != nil {
			return AccountSnapshot{}, fmt.Errorf("deposit: %w", err)
		}
		return c.fetchSnapshot(ctx, tx, userID)
	})
}

func (c *PostgresCashier) Withdraw(ctx context.Context, userID, idempotencyKey string, amount int64) (AccountSnapshot, error) {
	if amount <= 0 {
		return AccountSnapshot{}, fmt.Errorf("amount must be > 0")
	}
	if idempotencyKey == "" {
		return AccountSnapshot{}, fmt.Errorf("idempotency_key is required")
	}
	return c.withIdempotency(ctx, idempotencyKey, func(tx pgx.Tx) (AccountSnapshot, error) {
		var grossBalance, escrowed, version int64
		err := tx.QueryRow(ctx,
			`SELECT gross_balance, escrowed, version FROM accounts WHERE user_id = $1 FOR UPDATE`,
			userID).Scan(&grossBalance, &escrowed, &version)
		if errors.Is(err, pgx.ErrNoRows) {
			return AccountSnapshot{}, &UnknownUserError{Msg: "user not found: " + userID}
		}
		if err != nil {
			return AccountSnapshot{}, fmt.Errorf("withdraw select: %w", err)
		}

		if grossBalance-escrowed < amount {
			return AccountSnapshot{}, &InsufficientFundsError{
				Msg: fmt.Sprintf("insufficient funds: available=%d requested=%d", grossBalance-escrowed, amount),
			}
		}

		tag, err := tx.Exec(ctx,
			`UPDATE accounts SET gross_balance = $1, version = version + 1, updated_at = clock_timestamp()
			 WHERE user_id = $2 AND version = $3`,
			grossBalance-amount, userID, version)
		if err != nil {
			return AccountSnapshot{}, fmt.Errorf("withdraw update: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return AccountSnapshot{}, fmt.Errorf("concurrent modification for user: %s", userID)
		}
		return c.fetchSnapshot(ctx, tx, userID)
	})
}

func (c *PostgresCashier) Escrow(ctx context.Context, userID string, orderID uint32, amount int64) (AccountSnapshot, error) {
	if amount <= 0 {
		return AccountSnapshot{}, fmt.Errorf("amount must be > 0")
	}

	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return AccountSnapshot{}, err
	}
	defer tx.Rollback(ctx)

	var grossBalance, escrowed, version int64
	err = tx.QueryRow(ctx,
		`SELECT gross_balance, escrowed, version FROM accounts WHERE user_id = $1 FOR UPDATE`,
		userID).Scan(&grossBalance, &escrowed, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountSnapshot{}, &UnknownUserError{Msg: "user not found: " + userID}
	}
	if err != nil {
		return AccountSnapshot{}, fmt.Errorf("escrow select: %w", err)
	}

	if grossBalance-escrowed < amount {
		return AccountSnapshot{}, &InsufficientFundsError{
			Msg: fmt.Sprintf("insufficient funds: available=%d requested=%d", grossBalance-escrowed, amount),
		}
	}

	newEscrowed := escrowed + amount
	tag, err := tx.Exec(ctx,
		`UPDATE accounts SET escrowed = $1, version = version + 1, updated_at = clock_timestamp()
		 WHERE user_id = $2 AND version = $3`,
		newEscrowed, userID, version)
	if err != nil {
		return AccountSnapshot{}, fmt.Errorf("escrow update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return AccountSnapshot{}, fmt.Errorf("concurrent modification for user: %s", userID)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO escrow_entries (order_id, user_id, amount) VALUES ($1, $2, $3)`,
		orderID, userID, amount)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Duplicate order_id — idempotent retry; return current snapshot.
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				return AccountSnapshot{}, rbErr
			}
			return c.CheckAvailable(ctx, userID)
		}
		return AccountSnapshot{}, fmt.Errorf("escrow insert: %w", err)
	}

	snap := AccountSnapshot{GrossBalance: grossBalance, Escrowed: newEscrowed}
	if err := tx.Commit(ctx); err != nil {
		return AccountSnapshot{}, err
	}
	return snap, nil
}

func (c *PostgresCashier) ReleaseEscrow(ctx context.Context, orderID uint32) (AccountSnapshot, error) {
	// Unlocked pre-read to discover user_id and amount.
	var userID string
	var escrowAmount int64
	err := c.pool.QueryRow(ctx,
		`SELECT user_id, amount FROM escrow_entries WHERE order_id = $1`,
		orderID).Scan(&userID, &escrowAmount)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountSnapshot{}, &UnknownEscrowError{Msg: fmt.Sprintf("escrow not found for order_id: %d", orderID)}
	}
	if err != nil {
		return AccountSnapshot{}, fmt.Errorf("release_escrow pre-read: %w", err)
	}

	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return AccountSnapshot{}, err
	}
	defer tx.Rollback(ctx)

	// Delete escrow entry; re-check it exists (race between two release calls).
	tag, err := tx.Exec(ctx,
		`DELETE FROM escrow_entries WHERE order_id = $1`, orderID)
	if err != nil {
		return AccountSnapshot{}, fmt.Errorf("release_escrow delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return AccountSnapshot{}, &UnknownEscrowError{Msg: fmt.Sprintf("escrow not found for order_id: %d", orderID)}
	}

	var grossBalance, escrowed, version int64
	err = tx.QueryRow(ctx,
		`SELECT gross_balance, escrowed, version FROM accounts WHERE user_id = $1 FOR UPDATE`,
		userID).Scan(&grossBalance, &escrowed, &version)
	if err != nil {
		return AccountSnapshot{}, fmt.Errorf("release_escrow account select: %w", err)
	}

	newEscrowed := escrowed - escrowAmount
	tag, err = tx.Exec(ctx,
		`UPDATE accounts SET escrowed = $1, version = version + 1, updated_at = clock_timestamp()
		 WHERE user_id = $2 AND version = $3`,
		newEscrowed, userID, version)
	if err != nil {
		return AccountSnapshot{}, fmt.Errorf("release_escrow update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return AccountSnapshot{}, fmt.Errorf("concurrent modification for user: %s", userID)
	}

	snap := AccountSnapshot{GrossBalance: grossBalance, Escrowed: newEscrowed}
	if err := tx.Commit(ctx); err != nil {
		return AccountSnapshot{}, err
	}
	return snap, nil
}

func (c *PostgresCashier) CheckAvailable(ctx context.Context, userID string) (AccountSnapshot, error) {
	var snap AccountSnapshot
	err := c.pool.QueryRow(ctx,
		`SELECT gross_balance, escrowed FROM accounts WHERE user_id = $1`, userID).
		Scan(&snap.GrossBalance, &snap.Escrowed)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountSnapshot{}, nil
	}
	if err != nil {
		return AccountSnapshot{}, fmt.Errorf("check_available: %w", err)
	}
	return snap, nil
}

// withIdempotency wraps fn in a transaction with idempotency key deduplication.
func (c *PostgresCashier) withIdempotency(ctx context.Context, key string, fn func(pgx.Tx) (AccountSnapshot, error)) (AccountSnapshot, error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return AccountSnapshot{}, err
	}
	defer tx.Rollback(ctx)

	// Lock the idempotency key row (or non-existent row) to serialize retries.
	var rawResp []byte
	var expiresAt time.Time
	err = tx.QueryRow(ctx,
		`SELECT response, expires_at FROM idempotency_keys WHERE key = $1 FOR UPDATE`, key).
		Scan(&rawResp, &expiresAt)

	if err == nil {
		// Key exists — return cached response if not expired.
		if time.Now().Before(expiresAt) {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				return AccountSnapshot{}, rbErr
			}
			var cached pb.CashierResponse
			if err := proto.Unmarshal(rawResp, &cached); err != nil {
				return AccountSnapshot{}, fmt.Errorf("unmarshal cached response: %w", err)
			}
			return AccountSnapshot{GrossBalance: cached.GrossBalance, Escrowed: cached.Escrowed}, nil
		}
		// Expired — delete and re-execute.
		if _, err := tx.Exec(ctx, `DELETE FROM idempotency_keys WHERE key = $1`, key); err != nil {
			return AccountSnapshot{}, err
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return AccountSnapshot{}, fmt.Errorf("idempotency select: %w", err)
	}

	snap, err := fn(tx)
	if err != nil {
		return AccountSnapshot{}, err
	}

	// Persist the response so retries get the same answer.
	encoded, err := proto.Marshal(&pb.CashierResponse{
		GrossBalance: snap.GrossBalance,
		Escrowed:     snap.Escrowed,
	})
	if err != nil {
		return AccountSnapshot{}, fmt.Errorf("marshal idempotency response: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO idempotency_keys (key, response, expires_at) VALUES ($1, $2, $3)
		 ON CONFLICT (key) DO UPDATE SET response = $2, expires_at = $3`,
		key, encoded, time.Now().Add(c.idempotencyTTL))
	if err != nil {
		return AccountSnapshot{}, fmt.Errorf("idempotency insert: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return AccountSnapshot{}, err
	}
	return snap, nil
}

func (c *PostgresCashier) fetchSnapshot(ctx context.Context, tx pgx.Tx, userID string) (AccountSnapshot, error) {
	var snap AccountSnapshot
	err := tx.QueryRow(ctx,
		`SELECT gross_balance, escrowed FROM accounts WHERE user_id = $1`, userID).
		Scan(&snap.GrossBalance, &snap.Escrowed)
	if err != nil {
		return AccountSnapshot{}, fmt.Errorf("fetch snapshot: %w", err)
	}
	return snap, nil
}
