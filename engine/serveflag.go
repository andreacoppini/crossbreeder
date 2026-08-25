package main

import (
	"flag"
	"fmt"
	"os"
)

// serveFlag lets -serve be given three ways, because the directory is usually
// the one the tool was started in and saying so is redundant:
//
//	-serve                 share the working directory
//	-serve=C:\firmware     share that directory
//	-serve C:\firmware     the same, rescued from the leftover arguments
//
// Go's flag package cannot express "optional value" directly: declaring
// IsBoolFlag makes the bare form legal but stops the space form from consuming
// the next argument, so resolveServeDir picks it back up afterwards.
type serveFlag struct {
	set bool
	dir string
}

func (s *serveFlag) String() string { return s.dir }

func (s *serveFlag) IsBoolFlag() bool { return true }

func (s *serveFlag) Set(v string) error {
	switch v {
	case "true", "":
		s.set = true
	case "false":
		s.set = false
		s.dir = ""
	default:
		s.set = true
		s.dir = v
	}
	return nil
}

// resolveServeDir settles which directory to share and returns any arguments
// that were not consumed.
func resolveServeDir(s serveFlag, args []string, wd string) (dir string, rest []string, err error) {
	if !s.set {
		return "", args, nil
	}
	if s.dir != "" {
		return s.dir, args, nil
	}
	// "-serve C:\firmware": the path did not attach to the flag, so it is
	// sitting in the leftovers. Taking it is better than quietly sharing the
	// working directory and leaving the operator to wonder why.
	if len(args) > 0 {
		candidate := args[0]
		info, statErr := os.Stat(candidate)
		if statErr != nil {
			return "", args, fmt.Errorf("-serve %s: %w", candidate, statErr)
		}
		if !info.IsDir() {
			return "", args, fmt.Errorf("-serve %s: not a directory", candidate)
		}
		return candidate, args[1:], nil
	}
	return wd, args, nil
}

func workingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

var _ flag.Value = (*serveFlag)(nil)
