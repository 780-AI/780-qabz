// Package cmd dispatches the command line to an entry point.
package cmd

import (
	"fmt"
	"os"

	migratecmd "qabz/cmd/migrate"
	startapp "qabz/cmd/start-app"
)

// Run picks the entry point from args (without the program name). No argument or
// "start-app" starts the service; "migrate ..." runs a migration action.
func Run(args []string) int {
	if len(args) == 0 || args[0] == "start-app" {
		return startapp.Run()
	}
	if args[0] == "migrate" {
		return migratecmd.Run(args[1:])
	}
	fmt.Fprintf(os.Stderr, "unknown command %q; use start-app or migrate\n", args[0])
	return 2
}
