package config

import (
	"errors"
	"strings"
)

// SplitArgs finds the required --config argument (--config=path or --config path, with one or two
// dashes) anywhere in args. It returns the path and the other arguments in order.
func SplitArgs(args []string) (path string, rest []string, err error) {
	found := false
	for i := 0; i < len(args); i++ {
		a := strings.TrimPrefix(strings.TrimPrefix(args[i], "-"), "-")
		switch {
		case a == "config" && strings.HasPrefix(args[i], "-"):
			if i+1 >= len(args) {
				return "", nil, errors.New("--config needs a path")
			}
			i++
			path, found = args[i], true
		case strings.HasPrefix(a, "config=") && strings.HasPrefix(args[i], "-"):
			path, found = strings.TrimPrefix(a, "config="), true
		default:
			rest = append(rest, args[i])
		}
	}
	if !found || path == "" {
		return "", rest, errors.New("--config=path/to/config.yml is required")
	}
	return path, rest, nil
}

// Boot loads the Config named by --config in args. It returns the other arguments.
// A missing argument, a missing or unreadable file, invalid YAML or a broken rule panics: the
// App never starts with defaults.
func Boot(args []string) (Config, []string) {
	path, rest, err := SplitArgs(args)
	if err != nil {
		panic("config: " + err.Error())
	}
	c, err := LoadFile(path)
	if err != nil {
		panic("config: " + err.Error())
	}
	return c, rest
}
