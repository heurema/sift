package postgres

import (
	"context"
	"fmt"
	"strings"
)

func OpenHostedStore(ctx context.Context, dsn string) (*Store, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("postgres dsn is required")
	}

	store, err := Open(dsn)
	if err != nil {
		return nil, err
	}

	if err := store.Migrate(ctx); err != nil {
		store.Close()
		return nil, fmt.Errorf("migrate postgres database: %w", err)
	}

	return store, nil
}
