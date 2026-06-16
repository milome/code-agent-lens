package runtimepaths

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveUsesExplicitDBPathAndConfiguredDataDir(t *testing.T) {
	dataDir := filepath.Join("D:", "DevTools", "code-agent-lens", "data")
	dbPath := filepath.Join("D:", "Custom", "code-agent-lens.db")
	t.Setenv("CODE_AGENT_LENS_DATA_DIR", dataDir)
	t.Setenv("CODE_AGENT_LENS_DB_PATH", dbPath)

	paths := Resolve(func() (string, error) {
		return filepath.Join("C:", "Users", "milome"), nil
	})

	if paths.DataDir != dataDir {
		t.Fatalf("DataDir = %q, want configured data dir", paths.DataDir)
	}
	if paths.DBPath != dbPath {
		t.Fatalf("DBPath = %q, want explicit DB path", paths.DBPath)
	}
	if paths.ObservabilityDir != filepath.Join(dataDir, "observability") {
		t.Fatalf("ObservabilityDir = %q, want observability under configured data dir", paths.ObservabilityDir)
	}
}

func TestResolveWithOptionsIgnoresEnvironment(t *testing.T) {
	envDataDir := filepath.Join("D:", "Env", "data")
	envDBPath := filepath.Join("D:", "Env", "code-agent-lens.db")
	optionDataDir := filepath.Join("D:", "DevTools", "code-agent-lens", "data")
	optionDBPath := filepath.Join(optionDataDir, "code-agent-lens.db")
	t.Setenv("CODE_AGENT_LENS_DATA_DIR", envDataDir)
	t.Setenv("CODE_AGENT_LENS_DB_PATH", envDBPath)

	paths := ResolveWithOptions(func() (string, error) {
		return filepath.Join("C:", "Users", "milome"), nil
	}, Options{
		DataDir: optionDataDir,
		DBPath:  optionDBPath,
	})

	if paths.DataDir != optionDataDir {
		t.Fatalf("DataDir = %q, want explicit option data dir", paths.DataDir)
	}
	if paths.DBPath != optionDBPath {
		t.Fatalf("DBPath = %q, want explicit option db path", paths.DBPath)
	}
	if paths.ObservabilityDir != filepath.Join(optionDataDir, "observability") {
		t.Fatalf("ObservabilityDir = %q, want observability under explicit option data dir", paths.ObservabilityDir)
	}
}

func TestResolveUsesDataDirForDefaultDBPath(t *testing.T) {
	dataDir := filepath.Join("D:", "DevTools", "code-agent-lens", "data")
	t.Setenv("CODE_AGENT_LENS_DATA_DIR", dataDir)

	paths := Resolve(func() (string, error) {
		return filepath.Join("C:", "Users", "milome"), nil
	})

	if paths.DataDir != dataDir {
		t.Fatalf("DataDir = %q, want configured data dir", paths.DataDir)
	}
	if paths.DBPath != filepath.Join(dataDir, "code-agent-lens.db") {
		t.Fatalf("DBPath = %q, want db under configured data dir", paths.DBPath)
	}
	if paths.ObservabilityDir != filepath.Join(dataDir, "observability") {
		t.Fatalf("ObservabilityDir = %q, want observability under configured data dir", paths.ObservabilityDir)
	}
}

func TestResolveFallsBackToUserHome(t *testing.T) {
	homeDir := filepath.Join("C:", "Users", "milome")

	paths := Resolve(func() (string, error) {
		return homeDir, nil
	})

	wantDataDir := filepath.Join(homeDir, ".CodeAgentLens")
	if runtime.GOOS == "windows" {
		wantDataDir = CanonicalWindowsDataDir
	}
	if paths.DataDir != wantDataDir {
		t.Fatalf("DataDir = %q, want default user data dir", paths.DataDir)
	}
	if paths.DBPath != filepath.Join(wantDataDir, "code-agent-lens.db") {
		t.Fatalf("DBPath = %q, want db under default user data dir", paths.DBPath)
	}
	if paths.ObservabilityDir != filepath.Join(wantDataDir, "observability") {
		t.Fatalf("ObservabilityDir = %q, want observability under default user data dir", paths.ObservabilityDir)
	}
}
