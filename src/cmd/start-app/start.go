// Package startapp is the entry point of the service.
package startapp

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"qabz/app"
	"qabz/config"
)

// Run starts the service and blocks until SIGINT or SIGTERM. It returns the process exit code.
func Run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 2
	}
	b, err := app.NewDefaultBuilder(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "app:", err)
		return 1
	}
	a, err := b.Build()
	if err != nil {
		fmt.Fprintln(os.Stderr, "app:", err)
		return 1
	}
	if err := a.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "app:", err)
		return 1
	}
	return 0
}
