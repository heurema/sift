package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
)

func OpenStateStore(ctx context.Context, stateDir string) (*Store, error) {
	dbPath := filepath.Join(stateDir, "sift.db")

	store, err := Open(dbPath)
	if err != nil {
		return nil, err
	}

	if err := store.Migrate(ctx); err != nil {
		store.Close()
		return nil, fmt.Errorf("migrate sqlite database: %w", err)
	}

	return store, nil
}
