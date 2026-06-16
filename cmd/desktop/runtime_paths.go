package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/milome/code-agent-lens/internal/runtimepaths"
)

func resolveDesktopRuntimePaths(args []string, homeDirFunc func() (string, error)) runtimepaths.Paths {
	return runtimepaths.ResolveWithOptions(homeDirFunc, parseDesktopRuntimeOptions(args))
}

func parseDesktopRuntimeOptions(args []string) runtimepaths.Options {
	var opts runtimepaths.Options
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "--data-dir":
			if value, ok := nextDesktopArgValue(args, &i); ok {
				opts.DataDir = value
			}
		case strings.HasPrefix(arg, "--data-dir="):
			opts.DataDir = strings.TrimPrefix(arg, "--data-dir=")
		case arg == "--db-path":
			if value, ok := nextDesktopArgValue(args, &i); ok {
				opts.DBPath = value
			}
		case strings.HasPrefix(arg, "--db-path="):
			opts.DBPath = strings.TrimPrefix(arg, "--db-path=")
		}
	}
	return opts
}

func nextDesktopArgValue(args []string, index *int) (string, bool) {
	next := *index + 1
	if next >= len(args) || strings.HasPrefix(args[next], "--") {
		return "", false
	}
	*index = next
	return args[next], true
}

func ensureDesktopRuntimePaths(paths runtimepaths.Paths) error {
	if err := os.MkdirAll(paths.DataDir, 0755); err != nil {
		return fmt.Errorf("create data dir %s: %w", paths.DataDir, err)
	}

	dbDir := filepath.Dir(paths.DBPath)
	if dbDir == "" || dbDir == "." || dbDir == paths.DataDir {
		return nil
	}
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("create db dir %s: %w", dbDir, err)
	}
	return nil
}
