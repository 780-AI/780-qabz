// Package cmd dispatches the command line to an entry point.
package cmd

import (
	"fmt"
	"os"
	"strings"

	devdbcmd "qabz/cmd/devdb"
	migratecmd "qabz/cmd/migrate"
	startapp "qabz/cmd/start-app"
)

// Run picks the entry point from args (without the program name). No command, a first argument
// that is a flag, or "start-app" starts the service; "migrate ..." runs a migration action;
// "devdb seed|unseed" creates and seeds the bill tables, or unseeds them (development only).
// All take the required --config=path/to/config.yml.
func Run(args []string) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return startapp.Run(args)
	}
	if args[0] == "start-app" {
		return startapp.Run(args[1:])
	}
	if args[0] == "migrate" {
		return migratecmd.Run(args[1:])
	}
	if args[0] == "devdb" {
		return devdbcmd.Run(args[1:])
	}
	fmt.Fprintf(os.Stderr, "unknown command %q; use start-app, migrate or devdb\n", args[0])
	return 2
}
