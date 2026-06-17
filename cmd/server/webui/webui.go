package webui

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/milome/code-agent-lens/cmd/server/webui/api"
	"github.com/milome/code-agent-lens/internal/config"
	"github.com/milome/code-agent-lens/internal/proxy"
	"github.com/milome/code-agent-lens/internal/storage"
)

//go:embed ui
var uiFS embed.FS

// WebUI represents the web management interface
type WebUI struct {
	cfg        *config.Config
	apiHandler *api.Handler
}

// New creates a new WebUI instance
func New(cfg *config.Config, p *proxy.Proxy, storage *storage.SQLiteStorage) *WebUI {
	return &WebUI{
		cfg:        cfg,
		apiHandler: api.NewHandler(cfg, p, storage),
	}
}

// RegisterRoutes registers all web UI routes to the provided mux
func (w *WebUI) RegisterRoutes(mux *http.ServeMux) error {
	return w.RegisterRoutesWithAuth(mux, api.AuthConfig{
		Enabled:  w.cfg.BasicAuthEnabled,
		Username: w.cfg.BasicAuthUsername,
		Password: w.cfg.BasicAuthPassword,
	})
}

// RegisterRoutesWithAuth registers all web UI routes using the provided UI auth policy.
func (w *WebUI) RegisterRoutesWithAuth(mux *http.ServeMux, authConfig api.AuthConfig) error {
	w.apiHandler.SetAuth(authConfig)
	mux.HandleFunc("/api/", w.apiHandler.ServeHTTP)
	authMiddleware := api.BasicAuthMiddleware(authConfig)

	uiSubFS, err := fs.Sub(uiFS, "ui")
	if err != nil {
		return err
	}

	uiHandler := authMiddleware(http.FileServer(http.FS(uiSubFS)))
	mux.Handle("/ui/", http.StripPrefix("/ui/", uiHandler))

	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusFound)
	})

	return nil
}
