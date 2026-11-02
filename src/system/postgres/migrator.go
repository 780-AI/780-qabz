package postgres

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the pgx5 database driver
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// migrationsTable keeps the qabz state apart from the older bill service.
const migrationsTable = "qabz_schema_migrations"

// Migrator applies the embedded SQL files. golang-migrate takes a Postgres advisory lock
// on its own connection, so many containers can call Up at the same time: one applies,
// the others wait and then see no change.
type Migrator struct {
	dsn string
	src fs.FS
}

// NewMigrator takes a postgres:// DSN that points straight at Postgres (not a pooler in
// transaction mode, which breaks session locks) and the files.
func NewMigrator(dsn string, files fs.FS) *Migrator { return &Migrator{dsn: dsn, src: files} }

func (m *Migrator) open() (*migrate.Migrate, error) {
	d, err := iofs.New(m.src, ".")
	if err != nil {
		return nil, err
	}
	url := "pgx5://" + strings.TrimPrefix(m.dsn, "postgres://")
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	mg, err := migrate.NewWithSourceInstance("iofs", d, url+sep+"x-migrations-table="+migrationsTable)
	if err != nil {
		// The library can quote the URL: report no detail that holds the password.
		return nil, errors.New("migrate: cannot open the database")
	}
	return mg, nil
}

// Up applies all pending migrations. No pending migration is success. It returns when ctx ends,
// but a running migration is not cancelled halfway.
func (m *Migrator) Up(ctx context.Context) error {
	return m.run(ctx, func(mg *migrate.Migrate) error { return mg.Up() })
}

// Down rolls back n steps. It is for the manual migrate command only.
func (m *Migrator) Down(ctx context.Context, n int) error {
	return m.run(ctx, func(mg *migrate.Migrate) error { return mg.Steps(-n) })
}

// Force sets the version and clears the dirty flag. It is for the manual migrate command only.
func (m *Migrator) Force(version int) error {
	mg, err := m.open()
	if err != nil {
		return err
	}
	defer closeMigrate(mg)
	return mg.Force(version)
}

// Version returns the current version and the dirty flag. Version 0 means no migration ran.
func (m *Migrator) Version() (uint, bool, error) {
	mg, err := m.open()
	if err != nil {
		return 0, false, err
	}
	defer closeMigrate(mg)
	v, dirty, err := mg.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, nil
	}
	return v, dirty, err
}

func (m *Migrator) run(ctx context.Context, op func(*migrate.Migrate) error) error {
	mg, err := m.open()
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- op(mg) }()
	select {
	case err = <-done:
		closeMigrate(mg)
		if errors.Is(err, migrate.ErrNoChange) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
		return nil
	case <-ctx.Done():
		// Waiting too long. GracefulStop is honoured only between migrations, not during the lock wait,
		// so the goroutine below ends when the lock is granted or the process exits (start-app exits on this error).
		select {
		case mg.GracefulStop <- true:
		default:
		}
		go func() { <-done; closeMigrate(mg) }()
		return fmt.Errorf("migrate: %w", ctx.Err())
	}
}

func closeMigrate(mg *migrate.Migrate) { _, _ = mg.Close() }
