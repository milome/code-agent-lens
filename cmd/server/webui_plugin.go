package main

import (
	"net/http"

	"github.com/milome/code-agent-lens/cmd/server/webui"
	"github.com/milome/code-agent-lens/internal/config"
	"github.com/milome/code-agent-lens/internal/proxy"
	"github.com/milome/code-agent-lens/internal/storage"
)

// registerWebUI registers the Web UI routes
func registerWebUI(mux *http.ServeMux, cfg *config.Config, p *proxy.Proxy, storage *storage.SQLiteStorage) error {
	ui := webui.New(cfg, p, storage)
	return ui.RegisterRoutes(mux)
}
