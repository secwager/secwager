package internal

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewRefCache creates a background-refresh cache backed by Postgres.
func NewRefCache(pool *pgxpool.Pool) *RefCache {
	return newRefCache(pool)
}

// LoadRefFromDB performs a one-shot load of ref data from DB (used in tests).
func LoadRefFromDB(ctx context.Context, pool *pgxpool.Pool) (*refStore, error) {
	return loadRefFromDB(ctx, pool)
}

// NewPGStore is the exported entry point for main.
func NewPGStore(pool *pgxpool.Pool) InstrumentStore {
	return newPGStore(pool)
}
