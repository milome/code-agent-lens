package observability

import (
	"os"
	"strconv"
	"strings"
)

// Config controls CodeAgentLens local debug observability.
type Config struct {
	Enabled                bool
	LocalDebug             bool
	DumpEnabled            bool
	DumpDir                string
	ViewerEnabled          bool
	ViewerPublicURL        string
	CaptureHeaders         string
	CaptureBodies          string
	CaptureSecrets         bool
	CaptureStreamEvents    string
	MaxBodyBytes           int64
	PromptExtract          bool
	OTelPromptMode         string
	OTelPromptPreviewBytes int
	ServiceName            string
}

// LoadConfigFromEnv reads CodeAgentLens observability environment variables.
func LoadConfigFromEnv(defaultDumpDir string) Config {
	cfg := Config{
		DumpDir:                defaultDumpDir,
		ViewerPublicURL:        "http://127.0.0.1:3011/debug/obs",
		CaptureHeaders:         "none",
		CaptureBodies:          "none",
		CaptureStreamEvents:    "none",
		MaxBodyBytes:           0,
		OTelPromptMode:         "off",
		OTelPromptPreviewBytes: 4096,
		ServiceName:            "code-agent-lens",
	}

	cfg.Enabled = parseBoolEnv("CODE_AGENT_LENS_OTEL_ENABLED", false)
	cfg.LocalDebug = parseBoolEnv("CODE_AGENT_LENS_OBS_LOCAL_DEBUG", false)
	cfg.DumpEnabled = parseBoolEnv("CODE_AGENT_LENS_OBS_DUMP_ENABLED", false)
	cfg.ViewerEnabled = parseBoolEnv("CODE_AGENT_LENS_OBS_VIEWER_ENABLED", false)
	cfg.CaptureSecrets = parseBoolEnv("CODE_AGENT_LENS_OBS_CAPTURE_SECRETS", false)
	cfg.PromptExtract = parseBoolEnv("CODE_AGENT_LENS_OBS_PROMPT_EXTRACT", false)

	if v := strings.TrimSpace(os.Getenv("CODE_AGENT_LENS_OBS_DUMP_DIR")); v != "" {
		cfg.DumpDir = v
	}
	if v := strings.TrimSpace(os.Getenv("CODE_AGENT_LENS_OBS_VIEWER_PUBLIC_URL")); v != "" {
		cfg.ViewerPublicURL = strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(os.Getenv("CODE_AGENT_LENS_OBS_CAPTURE_HEADERS")); v != "" {
		cfg.CaptureHeaders = strings.ToLower(v)
	}
	if v := strings.TrimSpace(os.Getenv("CODE_AGENT_LENS_OBS_CAPTURE_BODIES")); v != "" {
		cfg.CaptureBodies = strings.ToLower(v)
	}
	if v := strings.TrimSpace(os.Getenv("CODE_AGENT_LENS_OBS_CAPTURE_STREAM_EVENTS")); v != "" {
		cfg.CaptureStreamEvents = strings.ToLower(v)
	}
	if v := strings.TrimSpace(os.Getenv("CODE_AGENT_LENS_OBS_OTEL_PROMPT_MODE")); v != "" {
		cfg.OTelPromptMode = strings.ToLower(v)
	}
	if v := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME")); v != "" {
		cfg.ServiceName = v
	}
	if v := strings.TrimSpace(os.Getenv("CODE_AGENT_LENS_OBS_MAX_BODY_BYTES")); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil && parsed >= 0 {
			cfg.MaxBodyBytes = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("CODE_AGENT_LENS_OBS_OTEL_PROMPT_PREVIEW_BYTES")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			cfg.OTelPromptPreviewBytes = parsed
		}
	}

	return cfg
}

func parseBoolEnv(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}
