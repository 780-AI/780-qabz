package main

import (
	"os"

	"qabz/cmd"
)

func main() { os.Exit(cmd.Run(os.Args[1:])) }
