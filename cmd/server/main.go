package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/milome/code-agent-lens/internal/config"
	"github.com/milome/code-agent-lens/internal/logger"
	"github.com/milome/code-agent-lens/internal/observability"
	"github.com/milome/code-agent-lens/internal/observability/viewer"
	"github.com/milome/code-agent-lens/internal/proxy"
	"github.com/milome/code-agent-lens/internal/storage"
)

const defaultViewerPort = 3011

func main() {
	// Parse command line flags
	portFlag := flag.Int("port", 0, "Force specific port (locked, cannot be changed via API)")
	flag.Parse()
	dataDir := resolveDataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		logger.Error("Failed to create data dir %s: %v", dataDir, err)
		os.Exit(1)
	}
	defaultDumpDir := filepath.Join(dataDir, "observability")
	obsCfg := observability.LoadConfigFromEnv(defaultDumpDir)
	obsRuntime, err := observability.Init(context.Background(), obsCfg, "headless")
	if err != nil {
		logger.Error("Failed to initialize observability runtime: %v", err)
		os.Exit(1)
	}
	defer shutdownObservability(obsRuntime)

	dbPath := os.Getenv("CODE_AGENT_LENS_DB_PATH")
	if dbPath == "" {
		dbPath = filepath.Join(dataDir, "code-agent-lens.db")
	}

	sqliteStorage, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		logger.Error("Failed to open SQLite storage: %v", err)
		os.Exit(1)
	}
	defer sqliteStorage.Close()

	cfg, err := loadConfig(sqliteStorage)
	if err != nil {
		logger.Error("Unable to load configuration: %v", err)
		os.Exit(1)
	}

	// Handle -port CLI flag (overrides config and locks port)
	if *portFlag > 0 {
		cfg.Port = *portFlag
		cfg.LockPort()
		logger.Info("Port locked to %d via CLI flag", *portFlag)
	}

	if cfg.BasicAuthEnabled && cfg.BasicAuthPassword == "" {
		randomPassword := generateRandomPassword(16)
		cfg.BasicAuthPassword = randomPassword
		logger.Info("======================================")
		logger.Info("  Basic Auth 密码已随机生成")
		logger.Info("  用户名: %s", cfg.BasicAuthUsername)
		logger.Info("  密码: %s", randomPassword)
		logger.Info("  请妥善保存，密码不会再次显示")
		logger.Info("======================================")
		adapter := storage.NewConfigStorageAdapter(sqliteStorage)
		_ = cfg.SaveToStorage(adapter)
	} else if cfg.BasicAuthEnabled {
		logger.Info("Basic Auth 已启用，用户名: %s", cfg.BasicAuthUsername)
	}

	applyEnvOverrides(cfg)
	setLogLevels(cfg.GetLogLevel())

	if err := cfg.Validate(); err != nil {
		logger.Error("Invalid configuration: %v", err)
		os.Exit(1)
	}

	deviceID, err := sqliteStorage.GetOrCreateDeviceID()
	if err != nil {
		logger.Warn("Failed to get device ID: %v, using default", err)
		deviceID = "default"
	}

	statsAdapter := storage.NewStatsStorageAdapter(sqliteStorage)
	p := proxy.New(cfg, statsAdapter, sqliteStorage, deviceID)
	p.SetObservabilityRuntime(obsRuntime)

	portalServer, err := startDebugPortalServer(obsCfg, obsRuntime, resolveViewerPort())
	if err != nil {
		logger.Error("Failed to start Debug Portal: %v", err)
		os.Exit(1)
	}
	defer shutdownHTTPServer(portalServer, "Debug Portal")

	// Create HTTP mux
	mux := http.NewServeMux()
	blockDebugViewerOnGateway(mux)
	if os.Getenv("CODE_AGENT_LENS_OBS_SMOKE_UPSTREAM") == "true" {
		mux.HandleFunc("/__obs_smoke/v1/responses", handleObservabilitySmokeUpstream)
		mux.HandleFunc("/__obs_smoke/v1/chat/completions", handleObservabilitySmokeUpstream)
		mux.HandleFunc("/__obs_smoke/v1/messages", handleObservabilitySmokeUpstream)
		cfg.UpdateEndpoints([]config.Endpoint{{
			Name:        "observability-smoke",
			APIUrl:      "http://127.0.0.1:" + strconv.Itoa(cfg.GetPort()) + "/__obs_smoke",
			APIKey:      "local-smoke-placeholder",
			AuthMode:    config.AuthModeAPIKey,
			Enabled:     true,
			Transformer: "openai2",
			Model:       observabilitySmokeModel(),
			Remark:      "local observability smoke upstream",
		}})
		cfg.BasicAuthEnabled = false
	}
	p.SetHTTPHandlerWrapper(viewer.Guard)

	// Initialize and register Web UI (optional plugin)
	// If webui package is not available, this will be skipped at compile time
	if err := registerWebUI(mux, cfg, p, sqliteStorage); err != nil {
		logger.Warn("Web UI not available: %v", err)
	} else {
		logger.Info("Web UI available at /ui/")
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- p.StartWithMux(mux)
	}()

	logger.Info("CodeAgentLens headless API listening on :%d (data dir: %s, db: %s)", cfg.GetPort(), dataDir, dbPath)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Info("Received signal %s, shutting down", sig.String())
		if err := p.Stop(); err != nil {
			logger.Warn("Graceful shutdown failed: %v", err)
		}
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Proxy server stopped with error: %v", err)
			os.Exit(1)
		}
	}

	logger.Info("CodeAgentLens stopped")
}

func shutdownObservability(rt *observability.Runtime) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rt.Shutdown(ctx); err != nil {
		logger.Warn("Observability shutdown failed: %v", err)
	}
}

func startDebugPortalServer(obsCfg observability.Config, obsRuntime *observability.Runtime, port int) (*http.Server, error) {
	if !obsCfg.ViewerEnabled {
		return nil, nil
	}
	mux := http.NewServeMux()
	viewer.Register(mux, obsCfg.DumpDir, true)
	handler := http.Handler(mux)
	handler = viewer.Guard(handler)
	if obsRuntime != nil {
		handler = obsRuntime.WrapHandler(handler, "code-agent-lens.portal")
	}
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      10 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return nil, err
	}
	go func() {
		logger.Info("CodeAgentLens Debug Portal listening on :%d", port)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Debug Portal stopped with error: %v", err)
		}
	}()
	return server, nil
}

func shutdownHTTPServer(server *http.Server, name string) {
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Warn("%s shutdown failed: %v", name, err)
	}
}

func resolveViewerPort() int {
	if raw := os.Getenv("CODE_AGENT_LENS_OBS_VIEWER_PORT"); raw != "" {
		if port, err := strconv.Atoi(raw); err == nil && port > 0 && port <= 65535 {
			return port
		}
		logger.Warn("Invalid CODE_AGENT_LENS_OBS_VIEWER_PORT value %q", raw)
	}
	return defaultViewerPort
}

func blockDebugViewerOnGateway(mux *http.ServeMux) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}
	mux.HandleFunc("/debug/obs", handler)
	mux.HandleFunc("/debug/obs/", handler)
}

func resolveDataDir() string {
	if dir := os.Getenv("CODE_AGENT_LENS_DATA_DIR"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".CodeAgentLens")
	}
	return "/data"
}

func loadConfig(sqliteStorage *storage.SQLiteStorage) (*config.Config, error) {
	adapter := storage.NewConfigStorageAdapter(sqliteStorage)
	cfg, err := config.LoadFromStorage(adapter)
	if err != nil {
		logger.Warn("Failed to load config from storage, using default: %v", err)
		cfg = config.DefaultConfig()
		if saveErr := cfg.SaveToStorage(adapter); saveErr != nil {
			logger.Warn("Failed to persist default config: %v", saveErr)
		}
	}

	// Seed a default endpoint when none are configured to avoid boot failure
	if len(cfg.Endpoints) == 0 {
		logger.Warn("No endpoints found; seeding a default endpoint")
		cfg.Endpoints = config.DefaultConfig().Endpoints
		if saveErr := cfg.SaveToStorage(adapter); saveErr != nil {
			logger.Warn("Failed to persist seeded endpoint: %v", saveErr)
		}
	}
	return cfg, nil
}

func applyEnvOverrides(cfg *config.Config) {
	if portStr := os.Getenv("CODE_AGENT_LENS_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			cfg.UpdatePort(port)
		} else {
			logger.Warn("Invalid CODE_AGENT_LENS_PORT value %q: %v", portStr, err)
		}
	}

	if levelStr := os.Getenv("CODE_AGENT_LENS_LOG_LEVEL"); levelStr != "" {
		if level, err := strconv.Atoi(levelStr); err == nil {
			cfg.UpdateLogLevel(level)
		} else {
			logger.Warn("Invalid CODE_AGENT_LENS_LOG_LEVEL value %q: %v", levelStr, err)
		}
	}

	if authEnabled := os.Getenv("CODE_AGENT_LENS_BASIC_AUTH_ENABLED"); authEnabled != "" {
		enabled := authEnabled == "1" || authEnabled == "true"
		cfg.BasicAuthEnabled = enabled
	}

	if username := os.Getenv("CODE_AGENT_LENS_BASIC_AUTH_USERNAME"); username != "" {
		cfg.BasicAuthUsername = username
	}

	if password := os.Getenv("CODE_AGENT_LENS_BASIC_AUTH_PASSWORD"); password != "" {
		cfg.BasicAuthPassword = password
	}
}

func setLogLevels(level int) {
	if level < 0 {
		return
	}
	logger.GetLogger().SetMinLevel(logger.LogLevel(level))
	logger.GetLogger().SetConsoleLevel(logger.LogLevel(level))
}

func generateRandomPassword(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		fallback := make([]byte, length)
		for i := range fallback {
			fallback[i] = byte(i*7%26 + 'a')
		}
		return string(fallback)
	}
	return hex.EncodeToString(bytes)[:length]
}

func observabilitySmokeModel() string {
	if model := os.Getenv("CODE_AGENT_LENS_OBS_SMOKE_MODEL"); model != "" {
		return model
	}
	return "code-agent-lens-local-smoke"
}

func handleObservabilitySmokeUpstream(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(body, &req)
	if req.Stream {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		for _, event := range []struct {
			name string
			data string
		}{
			{
				name: "response.created",
				data: `{"type":"response.created","response":{"id":"code-agent-lens-otel-smoke","object":"response","status":"in_progress","usage":{"input_tokens":7,"output_tokens":0,"total_tokens":7}}}`,
			},
			{
				name: "response.output_item.added",
				data: `{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_code-agent-lens_otel_smoke","role":"assistant","status":"in_progress","content":[]}}`,
			},
			{
				name: "response.content_part.added",
				data: `{"type":"response.content_part.added","output_index":0,"content_index":0,"part":{"type":"output_text","text":""}}`,
			},
			{
				name: "response.output_text.delta",
				data: `{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"code-agent-lens-otel-smoke-response"}`,
			},
			{
				name: "response.output_text.done",
				data: `{"type":"response.output_text.done","output_index":0,"content_index":0,"text":"code-agent-lens-otel-smoke-response"}`,
			},
			{
				name: "response.content_part.done",
				data: `{"type":"response.content_part.done","output_index":0,"content_index":0,"part":{"type":"output_text","text":"code-agent-lens-otel-smoke-response"}}`,
			},
			{
				name: "response.output_item.done",
				data: `{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_code-agent-lens_otel_smoke","role":"assistant","status":"completed","content":[{"type":"output_text","text":"code-agent-lens-otel-smoke-response"}]}}`,
			},
			{
				name: "response.completed",
				data: `{"type":"response.completed","response":{"id":"code-agent-lens-otel-smoke","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"code-agent-lens-otel-smoke-response"}]}],"usage":{"input_tokens":7,"output_tokens":11,"total_tokens":18}}}`,
			},
		} {
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.name, event.data)
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"id":"code-agent-lens-otel-smoke","object":"response","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"code-agent-lens-otel-smoke-response"}]}],"usage":{"input_tokens":7,"output_tokens":11,"total_tokens":18}}`))
}
