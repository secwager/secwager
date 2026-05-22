package internal

import "github.com/jackc/pgx/v5/pgxpool"

// LoadFixture is the exported entry point for main.
func LoadFixture(path string) (*refStore, error) {
	return loadFixture(path)
}

// NewPGStore is the exported entry point for main.
func NewPGStore(pool *pgxpool.Pool) InstrumentStore {
	return newPGStore(pool)
}
