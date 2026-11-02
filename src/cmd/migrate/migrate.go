// Package migratecmd is the manual migration command: up, down N, force V, version.
package migratecmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"qabz/config"
	"qabz/migrations"
	"qabz/system/postgres"
)

// Run executes one migration action and returns the process exit code.
// start-app already runs "up" at start; this command is for down, force and version.
func Run(args []string) int { return run(args, os.Stdout, os.Stderr) }

func run(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: migrate up | down N | force VERSION | version")
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(errOut, "config:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, cfg.MigrateTimeout)
	defer cancel()
	m := postgres.NewMigrator(cfg.PostgresDSN(), migrations.FS)

	switch args[0] {
	case "up":
		err = m.Up(ctx)
	case "down":
		n, perr := number(args, "down N")
		if perr != nil {
			fmt.Fprintln(errOut, perr)
			return 2
		}
		if n < 1 {
			fmt.Fprintln(errOut, "down needs N of at least 1")
			return 2
		}
		err = m.Down(ctx, n)
	case "force":
		n, perr := number(args, "force VERSION")
		if perr != nil {
			fmt.Fprintln(errOut, perr)
			return 2
		}
		err = m.Force(n)
	case "version":
		v, dirty, verr := m.Version()
		if verr != nil {
			err = verr
			break
		}
		fmt.Fprintf(out, "version=%d dirty=%t\n", v, dirty)
	default:
		fmt.Fprintf(errOut, "unknown action %q\n", args[0])
		return 2
	}
	if err != nil {
		fmt.Fprintln(errOut, "migrate:", err)
		return 1
	}
	return 0
}

func number(args []string, usage string) (int, error) {
	if len(args) != 2 {
		return 0, fmt.Errorf("usage: migrate %s", usage)
	}
	n, err := strconv.Atoi(args[1])
	if err != nil || n < 0 {
		return 0, fmt.Errorf("usage: migrate %s", usage)
	}
	return n, nil
}
