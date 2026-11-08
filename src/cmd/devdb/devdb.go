// Package devdbcmd is the development command that creates and seeds the bill tables (seed) or
// deletes the seed rows (unseed). It runs only with mode: dev.
package devdbcmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"qabz/config"
	"qabz/system/postgres"
	"qabz/system/postgres/devdb"
)

const usage = "usage: devdb --config=path/to/config.yml seed | unseed (development only: needs mode: dev)"

// database is the connection that the command uses. *pgxpool.Pool implements it.
type database interface {
	devdb.Beginner
	Close()
}

// opener connects to the database. Tests replace it to prove that a refused run never connects.
type opener func(ctx context.Context, dsn string) (database, error)

// Run executes seed or unseed and returns the process exit code: 0 done, 1 refused (mode is not
// dev) or failed, 2 usage error. A missing or unusable config file panics, as in the other commands;
// a config without mode is unusable (mode is required).
func Run(args []string) int { return run(args, os.Stdout, os.Stderr, openPostgres) }

func run(args []string, out, errOut io.Writer, open opener) int {
	if _, rest, _ := config.SplitArgs(args); len(rest) != 1 || (rest[0] != "seed" && rest[0] != "unseed") {
		fmt.Fprintln(errOut, usage)
		return 2
	}
	cfg, rest := config.Boot(args)
	// The guard runs before any connection: in stage or prod the command changes nothing.
	if cfg.Mode != config.ModeDev {
		fmt.Fprintf(errOut, "devdb: refused: runs only with mode: dev (mode is %q); nothing changed\n", cfg.Mode)
		return 1
	}
	if len(rest) != 1 {
		fmt.Fprintln(errOut, usage)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, cfg.MigrateTimeout)
	defer cancel()

	db, err := open(ctx, cfg.PostgresDSN())
	if err != nil {
		fmt.Fprintln(errOut, "devdb: connect:", err)
		return 1
	}
	if db == nil {
		fmt.Fprintln(errOut, "devdb: connect: no database")
		return 1
	}
	defer db.Close()

	var n devdb.Counts
	if rest[0] == "seed" {
		n, err = devdb.Seed(ctx, db)
	} else {
		n, err = devdb.Unseed(ctx, db)
	}
	if err != nil {
		fmt.Fprintf(errOut, "devdb %s: %v\n", rest[0], err)
		return 1
	}
	if rest[0] == "seed" {
		fmt.Fprintf(out, "devdb seed: bill tables ready on %s; inserted services=%d providers=%d (rows that existed are unchanged)\n",
			cfg.PostgresTarget(), n.Services, n.Providers)
	} else {
		fmt.Fprintf(out, "devdb unseed: deleted services=%d providers=%d on %s (the tables stay)\n",
			n.Services, n.Providers, cfg.PostgresTarget())
	}
	return 0
}

// openPostgres opens the pgx pool. pgx connects lazily: the first statement makes the connection.
func openPostgres(ctx context.Context, dsn string) (database, error) {
	c, err := postgres.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("no client")
	}
	return c.Pool(), nil
}
