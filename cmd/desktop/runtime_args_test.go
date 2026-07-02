package main

import (
	"path/filepath"
	"testing"
)

func TestResolveDesktopRuntimePathsUsesDataDirAndDBPathFlags(t *testing.T) {
	dataDir := filepath.Join("D:", "DevTools", "code-agent-lens", "data")
	dbPath := filepath.Join(dataDir, "code-agent-lens.db")

	paths := resolveDesktopRuntimePaths([]string{
		"--data-dir", dataDir,
		"--db-path=" + dbPath,
	}, func() (string, error) {
		return filepath.Join("C:", "Users", "milome"), nil
	})

	if paths.DataDir != dataDir {
		t.Fatalf("DataDir = %q, want data-dir flag", paths.DataDir)
	}
	if paths.DBPath != dbPath {
		t.Fatalf("DBPath = %q, want db-path flag", paths.DBPath)
	}
	if paths.ObservabilityDir != filepath.Join(dataDir, "observability") {
		t.Fatalf("ObservabilityDir = %q, want observability under data-dir flag", paths.ObservabilityDir)
	}
}

func TestResolveDesktopRuntimePathsIgnoresEnvironmentAndUnknownArgs(t *testing.T) {
	t.Setenv("CODE_AGENT_LENS_DATA_DIR", filepath.Join("D:", "Env", "data"))
	t.Setenv("CODE_AGENT_LENS_DB_PATH", filepath.Join("D:", "Env", "code-agent-lens.db"))

	dataDir := filepath.Join("D:", "DevTools", "code-agent-lens", "data")
	paths := resolveDesktopRuntimePaths([]string{
		"--ignored",
		"--data-dir=" + dataDir,
	}, func() (string, error) {
		return filepath.Join("C:", "Users", "milome"), nil
	})

	if paths.DataDir != dataDir {
		t.Fatalf("DataDir = %q, want data-dir flag", paths.DataDir)
	}
	if paths.DBPath != filepath.Join(dataDir, "code-agent-lens.db") {
		t.Fatalf("DBPath = %q, want default db under data-dir flag", paths.DBPath)
	}
}

func TestDesktopObservabilityDefaultsEnableOTel(t *testing.T) {
	t.Setenv("CODE_AGENT_LENS_OTEL_ENABLED", "")
	t.Setenv("CODE_AGENT_LENS_OBS_LOCAL_DEBUG", "")
	t.Setenv("CODE_AGENT_LENS_OBS_DUMP_ENABLED", "")
	t.Setenv("CODE_AGENT_LENS_OBS_PROMPT_EXTRACT", "")

	cfg := desktopObservabilityConfig(t.TempDir())

	if !cfg.Enabled {
		t.Fatalf("desktop observability should enable OTel by default")
	}
	if !cfg.LocalDebug || !cfg.DumpEnabled || !cfg.PromptExtract {
		t.Fatalf("desktop observability should default local debug capture on: %+v", cfg)
	}
	if cfg.CaptureHeaders != "all" || cfg.CaptureBodies != "all" || cfg.CaptureStreamEvents != "all" {
		t.Fatalf("desktop observability should capture debug metadata by default: %+v", cfg)
	}
}

func TestDesktopObservabilityEnvCanDisableOTel(t *testing.T) {
	t.Setenv("CODE_AGENT_LENS_OTEL_ENABLED", "false")

	cfg := desktopObservabilityConfig(t.TempDir())

	if cfg.Enabled {
		t.Fatalf("desktop observability should honor explicit CODE_AGENT_LENS_OTEL_ENABLED=false")
	}
}

func TestDesktopObservabilityCaptureEnvOverridesDefaults(t *testing.T) {
	t.Setenv("CODE_AGENT_LENS_OBS_CAPTURE_HEADERS", "none")
	t.Setenv("CODE_AGENT_LENS_OBS_CAPTURE_BODIES", "none")
	t.Setenv("CODE_AGENT_LENS_OBS_CAPTURE_STREAM_EVENTS", "none")

	cfg := desktopObservabilityConfig(t.TempDir())

	if cfg.CaptureHeaders != "none" || cfg.CaptureBodies != "none" || cfg.CaptureStreamEvents != "none" {
		t.Fatalf("desktop observability should honor explicit capture env overrides: %+v", cfg)
	}
}
