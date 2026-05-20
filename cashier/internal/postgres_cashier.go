package internal

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresCashier struct {
	pool *pgxpool.Pool
}

func NewPostgresCashier(pool *pgxpool.Pool) *PostgresCashier {
	return &PostgresCashier{pool: pool}
}

func (c *PostgresCashier) Deposit(ctx context.Context, userID, idempotencyKey string, amount int64) (AccountSnapshot, error) {
	if amount <= 0 {
		return AccountSnapshot{}, fmt.Errorf("amount must be > 0")
	}
	if idempotencyKey == "" {
		return AccountSnapshot{}, fmt.Errorf("idempotency_key is required")
	}
	return c.withIdempotency(ctx, userID, idempotencyKey, func(tx pgx.Tx) (AccountSnapshot, error) {
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
	return c.withIdempotency(ctx, userID, idempotencyKey, func(tx pgx.Tx) (AccountSnapshot, error) {
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
// On a cache hit it returns the current account state (not the frozen post-op snapshot)
// so the caller always sees live balances, and sets IsReplay=true so they can distinguish
// a retry from a fresh execution.
func (c *PostgresCashier) withIdempotency(ctx context.Context, userID, key string, fn func(pgx.Tx) (AccountSnapshot, error)) (AccountSnapshot, error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return AccountSnapshot{}, err
	}
	defer tx.Rollback(ctx)

	// Anchor the key atomically. The loser of a concurrent first-timer race blocks
	// until the winner commits, then sees ErrNoRows from RETURNING (no row returned
	// means the conflict branch fired — key already exists).
	var inserted bool
	err = tx.QueryRow(ctx,
		`INSERT INTO idempotency_keys (key) VALUES ($1) ON CONFLICT (key) DO NOTHING RETURNING true`, key).
		Scan(&inserted)

	if errors.Is(err, pgx.ErrNoRows) {
		// Key already exists — this operation ran before. Return current account state.
		var snap AccountSnapshot
		if err = tx.QueryRow(ctx,
			`SELECT gross_balance, escrowed FROM accounts WHERE user_id = $1`, userID).
			Scan(&snap.GrossBalance, &snap.Escrowed); err != nil {
			return AccountSnapshot{}, fmt.Errorf("idempotency replay: %w", err)
		}
		snap.IsReplay = true
		return snap, nil
	}
	if err != nil {
		return AccountSnapshot{}, fmt.Errorf("idempotency insert: %w", err)
	}

	snap, err := fn(tx)
	if err != nil {
		return AccountSnapshot{}, err // rolls back key INSERT too
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
