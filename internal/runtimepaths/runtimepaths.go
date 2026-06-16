package runtimepaths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const CanonicalWindowsDataDir = `D:\DevTools\code-agent-lens\data`

type Paths struct {
	DataDir          string
	DBPath           string
	ObservabilityDir string
}

type Options struct {
	DataDir string
	DBPath  string
}

func Resolve(homeDirFunc func() (string, error)) Paths {
	return ResolveWithOptions(homeDirFunc, Options{
		DataDir: os.Getenv("CODE_AGENT_LENS_DATA_DIR"),
		DBPath:  os.Getenv("CODE_AGENT_LENS_DB_PATH"),
	})
}

func ResolveWithOptions(homeDirFunc func() (string, error), opts Options) Paths {
	homeDir := ""
	if homeDirFunc != nil {
		if dir, err := homeDirFunc(); err == nil {
			homeDir = dir
		}
	}
	if homeDir == "" {
		homeDir = "."
	}

	dataDir := strings.TrimSpace(opts.DataDir)
	if dataDir == "" {
		dataDir = defaultDataDir(homeDir)
	}

	dbPath := strings.TrimSpace(opts.DBPath)
	if dbPath == "" {
		dbPath = filepath.Join(dataDir, "code-agent-lens.db")
	}

	return Paths{
		DataDir:          dataDir,
		DBPath:           dbPath,
		ObservabilityDir: filepath.Join(dataDir, "observability"),
	}
}

func defaultDataDir(homeDir string) string {
	if runtime.GOOS == "windows" {
		return CanonicalWindowsDataDir
	}
	return filepath.Join(homeDir, ".CodeAgentLens")
}
