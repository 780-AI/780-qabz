// Package devdb creates the tables of the bill service database and seeds or unseeds the services
// and providers rows. It is for development only: the devdb command calls it only with mode: dev.
// It is not a qabz migration: qabz does not own the bill tables (system.html, Issue #2).
package devdb

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var (
	//go:embed schema.sql
	schemaSQL string
	//go:embed seed_services.sql
	seedServicesSQL string
	//go:embed seed_providers.sql
	seedProvidersSQL string
	//go:embed unseed_services.sql
	unseedServicesSQL string
	//go:embed unseed_providers.sql
	unseedProvidersSQL string
)

// lockSQL serializes two devdb runs on one database: CREATE TABLE IF NOT EXISTS is not safe when two
// sessions run it at the same time. The lock ends with the transaction.
const lockSQL = `SELECT pg_advisory_xact_lock(780015)`

// sequenceSQL moves the id sequence of a table past its largest id, so a later insert without an id
// does not collide with a seeded id. It never moves a sequence back. A table with no id sequence
// (made before this command, by other DDL) or with no rows is skipped. %s is a fixed table name.
const sequenceSQL = `SELECT setval(s.seq, GREATEST(m.max_id, COALESCE(pg_sequence_last_value(s.seq), 0)))
FROM (SELECT pg_get_serial_sequence('%[1]s', 'id')::regclass AS seq) s,
     (SELECT max(id) AS max_id FROM %[1]s) m
WHERE s.seq IS NOT NULL AND m.max_id IS NOT NULL`

// Beginner starts a transaction. *pgxpool.Pool implements it.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Counts is the number of rows that a call inserted (Seed) or deleted (Unseed), per table.
type Counts struct {
	Services  int64
	Providers int64
}

// Seed creates every bill table that does not exist, inserts the seed rows whose id does not exist
// and moves the services and providers id sequences past the largest id. A table, an index or a row
// that exists is not changed, so a second call inserts nothing. All steps run in one transaction:
// on an error nothing is changed.
func Seed(ctx context.Context, db Beginner) (Counts, error) {
	var n Counts
	err := inTx(ctx, db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, schemaSQL); err != nil {
			return fmt.Errorf("create bill tables: %w", err)
		}
		var err error
		if n.Services, err = exec(ctx, tx, seedServicesSQL); err != nil {
			return fmt.Errorf("seed services: %w", err)
		}
		if n.Providers, err = exec(ctx, tx, seedProvidersSQL); err != nil {
			return fmt.Errorf("seed providers: %w", err)
		}
		for _, table := range []string{"services", "providers"} {
			if _, err := tx.Exec(ctx, fmt.Sprintf(sequenceSQL, table)); err != nil {
				return fmt.Errorf("%s id sequence: %w", table, err)
			}
		}
		return nil
	})
	if err != nil {
		return Counts{}, err
	}
	return n, nil
}

// Unseed deletes the seed rows from services and providers by id. The tables stay. A second call
// deletes nothing. Both deletes run in one transaction.
func Unseed(ctx context.Context, db Beginner) (Counts, error) {
	var n Counts
	err := inTx(ctx, db, func(tx pgx.Tx) error {
		var err error
		if n.Services, err = exec(ctx, tx, unseedServicesSQL); err != nil {
			return fmt.Errorf("unseed services: %w", err)
		}
		if n.Providers, err = exec(ctx, tx, unseedProvidersSQL); err != nil {
			return fmt.Errorf("unseed providers: %w", err)
		}
		return nil
	})
	if err != nil {
		return Counts{}, err
	}
	return n, nil
}

// exec runs one statement and returns the number of rows it changed.
func exec(ctx context.Context, tx pgx.Tx, sql string) (int64, error) {
	tag, err := tx.Exec(ctx, sql)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// inTx takes the devdb lock, runs fn and commits. On an error it rolls back.
func inTx(ctx context.Context, db Beginner, fn func(pgx.Tx) error) error {
	if db == nil {
		return fmt.Errorf("devdb: no database")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	if tx == nil {
		return fmt.Errorf("begin: no transaction")
	}
	// Rollback after a commit does nothing; it covers every early return.
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, lockSQL); err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
