package viewer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/milome/code-agent-lens/internal/observability/dump"
)

const (
	defaultPromptExplorerLimit = 50
	maxPromptExplorerLimit     = 200
	maxPromptSnippetBytes      = 512
	defaultCodexToolTurnLimit  = 20
	maxCodexToolCallSnippet    = 360
	newTabRelAttrs             = `target="_blank" rel="noopener noreferrer"`
)

type breadcrumbItem struct {
	Label string
	Href  string
}

type pageOptions struct {
	Title      string
	Active     string
	Breadcrumb []breadcrumbItem
}

type promptRoleSummary struct {
	Role    string
	File    string
	Bytes   int
	Snippet string
}

type requestPromptSummary struct {
	RequestID   string
	TraceID     string
	RunID       string
	PromptCount int
	TotalBytes  int
	Roles       []promptRoleSummary
}

type promptExplorerQuery struct {
	Q      string
	Role   string
	Run    string
	Limit  int
	Offset int
}

type portalQuery struct {
	Limit  int
	Offset int
	View   string
	Sort   string
	Q      string
	Run    string
	Role   string
}

type portalRecord struct {
	Record requestRecord
	Index  dump.PromptIndex
	Kind   string
}

type requestRecord struct {
	RunID     string `json:"run_id"`
	Timestamp string `json:"timestamp"`
	TraceID   string `json:"trace_id"`
	SpanID    string `json:"span_id"`
	RequestID string `json:"request_id"`
	ObsRef    string `json:"obs_ref"`
	Path      string `json:"path"`
	sequence  int64
}

type codexToolTurn struct {
	TurnID      string
	TraceID     string
	StartedAt   string
	SessionFile string
	Calls       []codexToolCall
}

type codexToolCall struct {
	Index       int
	Timestamp   string
	Name        string
	CallID      string
	Arguments   string
	Output      string
	OutputTime  string
	HasOutput   bool
	OutputState string
}

type server struct {
	root       string
	cacheMu    sync.Mutex
	indexCache map[string]cachedPromptIndex
}

type cachedPromptIndex struct {
	modUnixNano int64
	size        int64
	index       dump.PromptIndex
}

func Guard(next http.Handler) http.Handler {
	if next == nil {
		next = http.DefaultServeMux
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.EscapedPath(), "/debug/obs/") && unsafeDebugPath(r) {
			http.Error(w, "invalid debug viewer path", http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func Register(mux *http.ServeMux, dumpRoot string, enabled bool) {
	if !enabled || mux == nil {
		return
	}
	s := &server{root: dumpRoot}
	mux.HandleFunc("/debug/obs", s.handlePortal)
	mux.HandleFunc("/debug/obs/", s.handlePortal)
	mux.HandleFunc("/debug/obs/prompts", s.handleGlobalPrompts)
	mux.HandleFunc("/debug/obs/codex/tool-calls", s.handleCodexToolCalls)
	mux.HandleFunc("/debug/obs/tool/jaeger", s.handleToolJaeger)
	mux.HandleFunc("/debug/obs/tool/grafana", s.handleToolGrafana)
	mux.HandleFunc("/debug/obs/trace/", s.handleTrace)
	mux.HandleFunc("/debug/obs/ref/", s.handleRef)
	mux.HandleFunc("/debug/obs/request", s.handleRequest)
	mux.HandleFunc("/debug/obs/request/", s.handleRequest)
}

func unsafeDebugPath(r *http.Request) bool {
	raw := r.URL.EscapedPath()
	unescaped := r.URL.Path
	if strings.Contains(raw, "..") || strings.Contains(unescaped, "..") {
		return true
	}
	if strings.Contains(raw, "%2e") || strings.Contains(raw, "%2E") {
		return true
	}
	return false
}

func (s *server) handleTrace(w http.ResponseWriter, r *http.Request) {
	traceID := strings.TrimPrefix(r.URL.Path, "/debug/obs/trace/")
	if !safeID(traceID) {
		http.Error(w, "invalid trace id", http.StatusBadRequest)
		return
	}
	records, err := s.findRecords(func(rec requestRecord) bool { return rec.TraceID == traceID })
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(records) == 0 {
		http.NotFound(w, r)
		return
	}
	s.writeOverview(w, records, pageOptions{
		Title:  "Trace " + traceID,
		Active: "debug",
		Breadcrumb: []breadcrumbItem{
			{Label: "Portal", Href: "/debug/obs"},
			{Label: "Trace", Href: "/debug/obs/trace/" + traceID},
		},
	})
}

func (s *server) handlePortal(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/debug/obs" && r.URL.Path != "/debug/obs/" {
		http.NotFound(w, r)
		return
	}
	records, err := s.findRecords(func(requestRecord) bool { return true })
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	query := parsePortalQuery(r.URL.Query())
	portalRecords := s.buildPortalRecords(records)
	sortPortalRecords(portalRecords, query.Sort)
	promptRecords := filterPortalRecordsByKind(portalRecords, "prompt")
	errorRecords := filterPortalRecordsByKind(portalRecords, "error")
	noiseRecords := filterPortalRecordsByKind(portalRecords, "noise")
	llmRecords := filterPortalRecordsByKinds(portalRecords, "prompt", "error")
	rawSelectedRecords, sectionTitle, sectionDescription, emptyState := selectPortalRecordsForView(query.View, promptRecords, errorRecords, llmRecords, noiseRecords)
	selectedRecords := filterPortalRecords(rawSelectedRecords, query)
	sessionPage := paginatePortalRecords(selectedRecords, query.Limit, query.Offset)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var b strings.Builder
	writePageStart(&b, pageOptions{
		Title:      "CodeAgentLens Debug Portal",
		Active:     "portal",
		Breadcrumb: []breadcrumbItem{{Label: "Portal", Href: "/debug/obs"}},
	})
	b.WriteString("<section class=\"hero dashboard-hero\"><p class=\"eyebrow\">Local Observability</p><h1>CodeAgentLens Debug Portal</h1><p>Grafana-style control plane for local traces, prompt captures, and Docker observability tools.</p></section>")

	b.WriteString("<section class=\"status-grid\" aria-label=\"Status tiles\">")
	writeStatusTile(&b, "Health", "healthy endpoint", "/health", true)
	writeStatusTile(&b, "Prompt Sessions", strconv.Itoa(len(promptRecords)), "/debug/obs", false)
	writeStatusTile(&b, "LLM API Errors", strconv.Itoa(len(errorRecords)), "/debug/obs?view=errors", false)
	writeStatusTile(&b, "Filtered Noise", strconv.Itoa(len(noiseRecords)), "/debug/obs?view=noise", false)
	writeStatusTile(&b, "Raw Total", strconv.Itoa(len(portalRecords)), "/debug/obs?view=all", false)
	b.WriteString("</section>")

	b.WriteString("<section class=\"dashboard-grid\">")
	b.WriteString("<section class=\"panel\"><div class=\"section-heading\"><p class=\"eyebrow\">Operate</p><h2>Primary Actions</h2></div><div class=\"action-grid\">")
	writeToolCard(&b, toolCard{Title: "Debug Viewer", Desc: "Open request and trace records", Href: "/debug/obs", Code: "/debug/obs"})
	writeToolCard(&b, toolCard{Title: "Prompt Explorer", Desc: "Search prompt request summaries", Href: "/debug/obs/prompts", Code: "/debug/obs/prompts"})
	writeToolCard(&b, toolCard{Title: "Codex Tool Chain", Desc: "Rebuild CLI tool calls from rollout JSONL", Href: "/debug/obs/codex/tool-calls", Code: "/debug/obs/codex/tool-calls"})
	writeToolCard(&b, toolCard{Title: "Jaeger Wrapper", Desc: "Keep Portal navigation around Jaeger", Href: "/debug/obs/tool/jaeger", Code: "/debug/obs/tool/jaeger"})
	writeToolCard(&b, toolCard{Title: "Grafana Wrapper", Desc: "Keep Portal navigation around Grafana", Href: "/debug/obs/tool/grafana", Code: "/debug/obs/tool/grafana"})
	b.WriteString("</div></section>")

	b.WriteString("<section class=\"panel\" id=\"docker-stack\"><div class=\"section-heading\"><p class=\"eyebrow\">External</p><h2>Docker Observability Stack</h2></div><div class=\"action-grid\">")
	for _, item := range []struct {
		title string
		desc  string
		href  string
	}{
		{"Jaeger", "Trace 搜索与链路查看", "http://127.0.0.1:16686"},
		{"Grafana", "Dashboard 与 Tempo 查询", "http://127.0.0.1:13000"},
		{"Prometheus", "Metrics 查询页面", "http://127.0.0.1:9090/graph"},
		{"Tempo", "Tempo 状态页面", "http://127.0.0.1:3200/status"},
		{"OTel Collector", "Collector 自身 metrics 页面", "http://127.0.0.1:8888/metrics"},
	} {
		writeToolCard(&b, toolCard{Title: item.title, Desc: item.desc, Href: item.href, Code: item.href, NewTab: true})
	}
	b.WriteString("</div></section>")
	b.WriteString("</section>")

	b.WriteString("<section class=\"panel\"><div class=\"section-heading\"><p class=\"eyebrow\">Latest</p><h2>" + html.EscapeString(sectionTitle) + "</h2></div>")
	b.WriteString("<p class=\"muted\">" + html.EscapeString(sectionDescription) + " Showing " + strconv.Itoa(len(sessionPage)) + " of " + strconv.Itoa(len(selectedRecords)) + " sessions, " + html.EscapeString(query.Sort) + " first. Raw total: " + strconv.Itoa(len(records)) + ".</p>")
	writePortalFilters(&b, query)
	if len(sessionPage) == 0 {
		b.WriteString("<p class=\"empty-state\">" + html.EscapeString(emptyState) + "</p>")
	} else {
		writePortalSessionTable(&b, sessionPage)
	}
	writePortalPagination(&b, query, len(selectedRecords))
	b.WriteString("</section>")

	b.WriteString("<section class=\"panel quick-links\"><h2>Non-shell links</h2><p>These open in a new tab so the Portal remains available.</p><p>")
	writeInlineLink(&b, "Health", "/health", true)
	b.WriteString(" ")
	writeInlineLink(&b, "Web UI", "/ui/", true)
	b.WriteString("</p></section>")
	writePageEnd(&b)
	_, _ = w.Write([]byte(b.String()))
}

func (s *server) handleGlobalPrompts(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/debug/obs/prompts" {
		http.NotFound(w, r)
		return
	}
	records, err := s.findRecords(func(requestRecord) bool { return true })
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sortRecordsNewestFirst(records)
	query := parsePromptExplorerQuery(r.URL.Query())
	summaries := s.buildPromptSummaries(records)
	filtered := filterPromptSummaries(summaries, query)
	page := paginatePromptSummaries(filtered, query)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var b strings.Builder
	writePageStart(&b, pageOptions{
		Title:      "Prompt Explorer",
		Active:     "prompts",
		Breadcrumb: []breadcrumbItem{{Label: "Portal", Href: "/debug/obs"}, {Label: "Prompts", Href: "/debug/obs/prompts"}},
	})
	b.WriteString("<section class=\"hero\"><p class=\"eyebrow\">Prompt Capture</p><h1>Prompt Explorer</h1><p>Jaeger-style trace list with hover previews. Click a trace or request to open the full prompt details.</p></section>")
	b.WriteString("<section class=\"explorer-layout\"><aside class=\"filter-rail\"><h2>Filters</h2>")
	b.WriteString("<form method=\"get\" action=\"/debug/obs/prompts\" class=\"filter-form\">")
	writeInput(&b, "q", "Search metadata", query.Q)
	writeInput(&b, "role", "Role", query.Role)
	writeInput(&b, "run", "Run", query.Run)
	writeInput(&b, "limit", "Limit", strconv.Itoa(query.Limit))
	writeInput(&b, "offset", "Offset", strconv.Itoa(query.Offset))
	b.WriteString("<button type=\"submit\">Apply filters</button><a class=\"reset-link\" href=\"/debug/obs/prompts\">Reset</a></form></aside>")
	b.WriteString("<section class=\"panel explorer-results\"><div class=\"section-heading\"><p class=\"eyebrow\">Results</p><h2>Prompt Requests</h2></div>")
	b.WriteString("<p class=\"muted\">Showing " + strconv.Itoa(len(page)) + " of " + strconv.Itoa(len(filtered)) + " prompt requests. Prompt-less requests are excluded.</p>")
	if len(page) == 0 {
		b.WriteString("<p class=\"empty-state\">No captured prompts found. Return to <a href=\"/debug/obs\">Portal</a> after generating LLM traffic.</p>")
	} else {
		writePromptTraceList(&b, page)
	}
	writePagination(&b, query, len(filtered))
	b.WriteString("</section></section>")
	writePageEnd(&b)
	_, _ = w.Write([]byte(b.String()))
}

func (s *server) handleCodexToolCalls(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/debug/obs/codex/tool-calls" {
		http.NotFound(w, r)
		return
	}
	turns, sourceRoot, err := loadCodexToolTurns(defaultCodexToolTurnLimit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var b strings.Builder
	writePageStart(&b, pageOptions{
		Title:  "Codex Tool Call Chain",
		Active: "debug",
		Breadcrumb: []breadcrumbItem{
			{Label: "Portal", Href: "/debug/obs"},
			{Label: "Codex Tool Calls", Href: "/debug/obs/codex/tool-calls"},
		},
	})
	b.WriteString("<section class=\"hero\"><p class=\"eyebrow\">Codex CLI</p><h1>Codex Tool Call Chain</h1>")
	b.WriteString("<p>Reconstructs tool call order from local rollout JSONL. Use Jaeger links for span timing under service <code>codex_cli_rs</code>.</p>")
	b.WriteString("<p>source: <code>" + html.EscapeString(sourceRoot) + "</code></p></section>")
	if len(turns) == 0 {
		b.WriteString("<section class=\"panel\"><p class=\"empty-state\">No Codex tool calls found in recent rollout files.</p></section>")
		writePageEnd(&b)
		_, _ = w.Write([]byte(b.String()))
		return
	}
	writeCodexToolTurns(&b, turns)
	writePageEnd(&b)
	_, _ = w.Write([]byte(b.String()))
}

func (s *server) handleRef(w http.ResponseWriter, r *http.Request) {
	obsRef := strings.TrimPrefix(r.URL.Path, "/debug/obs/ref/")
	parts := strings.Split(obsRef, "/")
	if len(parts) != 2 || !safeID(parts[0]) || !safeID(parts[1]) {
		http.Error(w, "invalid obs ref", http.StatusBadRequest)
		return
	}
	records, err := s.findRecords(func(rec requestRecord) bool { return rec.TraceID == parts[0] && rec.RequestID == parts[1] })
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(records) == 0 {
		http.NotFound(w, r)
		return
	}
	s.writeOverview(w, records, pageOptions{
		Title:  "Reference " + obsRef,
		Active: "debug",
		Breadcrumb: []breadcrumbItem{
			{Label: "Portal", Href: "/debug/obs"},
			{Label: "Trace", Href: "/debug/obs/trace/" + parts[0]},
			{Label: "Request", Href: "/debug/obs/request/" + parts[1]},
		},
	})
}

func (s *server) handleRequest(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/debug/obs/request/")
	parts := strings.Split(rest, "/")
	if len(parts) == 1 {
		s.handleRequestOverview(w, r, parts[0])
		return
	}
	requestID, kind := parts[0], parts[1]
	if kind == "prompts" && len(parts) == 2 {
		if !safeID(requestID) {
			http.Error(w, "invalid request id", http.StatusBadRequest)
			return
		}
		s.serveAllPrompts(w, r, requestID)
		return
	}
	if kind == "prompt-preview" && len(parts) == 2 {
		if !safeID(requestID) {
			http.Error(w, "invalid request id", http.StatusBadRequest)
			return
		}
		s.servePromptPreview(w, r, requestID)
		return
	}
	if kind == "artifact" && len(parts) == 3 {
		if !safeID(requestID) {
			http.Error(w, "invalid request id", http.StatusBadRequest)
			return
		}
		s.serveArtifactViewer(w, r, requestID, parts[2])
		return
	}
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	name := strings.Join(parts[2:], "/")
	if !safeID(requestID) {
		http.Error(w, "invalid request id", http.StatusBadRequest)
		return
	}
	switch kind {
	case "prompt":
		s.servePrompt(w, r, requestID, name)
	case "body":
		s.serveManifestFile(w, r, requestID, name)
	case "stream":
		s.serveStream(w, r, requestID, name)
	default:
		http.NotFound(w, r)
	}
}

func (s *server) handleRequestOverview(w http.ResponseWriter, r *http.Request, requestID string) {
	if !safeID(requestID) {
		http.Error(w, "invalid request id", http.StatusBadRequest)
		return
	}
	records, err := s.findRecords(func(rec requestRecord) bool { return rec.RequestID == requestID })
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(records) == 0 {
		http.NotFound(w, r)
		return
	}
	s.writeOverview(w, records, pageOptions{
		Title:  "Request " + requestID,
		Active: "debug",
		Breadcrumb: []breadcrumbItem{
			{Label: "Portal", Href: "/debug/obs"},
			{Label: "Request", Href: "/debug/obs/request/" + requestID},
		},
	})
}

func (s *server) serveAllPrompts(w http.ResponseWriter, r *http.Request, requestID string) {
	rec, index, err := s.loadRequestIndex(requestID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var b strings.Builder
	writePageStart(&b, pageOptions{
		Title:  "All Prompts",
		Active: "prompts",
		Breadcrumb: []breadcrumbItem{
			{Label: "Portal", Href: "/debug/obs"},
			{Label: "Prompts", Href: "/debug/obs/prompts"},
			{Label: "Request", Href: "/debug/obs/request/" + rec.RequestID + "/prompts"},
		},
	})
	b.WriteString("<section class=\"hero\"><p class=\"eyebrow\">Prompt Capture</p><h1>All Prompts</h1>")
	b.WriteString("<p>request_id: " + html.EscapeString(rec.RequestID) + "</p>")
	b.WriteString("<p>trace_id: <a href=\"/debug/obs/trace/" + html.EscapeString(rec.TraceID) + "\">" + html.EscapeString(rec.TraceID) + "</a></p></section>")
	if len(index.Prompts) == 0 {
		b.WriteString("<section class=\"panel\"><p class=\"muted\">No prompts captured for this request. This usually means the request did not contain an LLM API body, or the body was empty.</p></section>")
		writePageEnd(&b)
		_, _ = w.Write([]byte(b.String()))
		return
	}
	for i, prompt := range index.Prompts {
		text := prompt.Text
		if text == "" && prompt.File != "" {
			var readErr error
			text, readErr = s.readManifestFile(rec, index, prompt.File)
			if readErr != nil {
				text = "unable to read prompt file: " + readErr.Error()
			}
		}
		b.WriteString("<section class=\"prompt-block\">")
		b.WriteString("<h2>" + html.EscapeString(prompt.Role) + "</h2>")
		b.WriteString("<p class=\"muted\">index: " + fmt.Sprintf("%d", i+1) + "</p>")
		if prompt.File != "" {
			b.WriteString("<p class=\"muted\">file: " + html.EscapeString(prompt.File) + "</p>")
			b.WriteString("<p><a href=\"/debug/obs/request/" + html.EscapeString(rec.RequestID) + "/prompt/" + html.EscapeString(prompt.Role) + "\" " + newTabRelAttrs + ">View raw</a></p>")
		}
		b.WriteString("<pre>" + html.EscapeString(text) + "</pre>")
		b.WriteString("</section>")
	}
	writePageEnd(&b)
	_, _ = w.Write([]byte(b.String()))
}

func (s *server) servePromptPreview(w http.ResponseWriter, r *http.Request, requestID string) {
	rec, index, err := s.loadRequestIndex(requestID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	summary := s.promptSummaryFromIndex(rec, index)
	summaries := []requestPromptSummary{summary}
	s.hydratePromptSummarySnippets(summaries)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var b strings.Builder
	writePromptHoverCardBody(&b, summaries[0])
	_, _ = w.Write([]byte(b.String()))
}

func (s *server) serveArtifactViewer(w http.ResponseWriter, r *http.Request, requestID, name string) {
	rec, index, err := s.loadRequestIndex(requestID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data, ok, err := s.readManifestBytes(rec, index, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var b strings.Builder
	writePageStart(&b, pageOptions{
		Title:  "Artifact Viewer",
		Active: "portal",
		Breadcrumb: []breadcrumbItem{
			{Label: "Portal", Href: "/debug/obs"},
			{Label: "Request", Href: "/debug/obs/request/" + rec.RequestID},
			{Label: "Artifact", Href: "/debug/obs/request/" + rec.RequestID + "/artifact/" + name},
		},
	})
	b.WriteString("<section class=\"hero\"><p class=\"eyebrow\">Manifest-bound file</p><h1>Artifact Viewer</h1>")
	b.WriteString("<p>request_id: <code>" + html.EscapeString(rec.RequestID) + "</code></p>")
	b.WriteString("<p>logical_name: <code>" + html.EscapeString(name) + "</code></p>")
	b.WriteString("<p><a href=\"/debug/obs/request/" + html.EscapeString(rec.RequestID) + "/body/" + html.EscapeString(name) + "\" " + newTabRelAttrs + ">View raw</a></p></section>")
	b.WriteString("<section class=\"panel\"><pre>" + html.EscapeString(string(data)) + "</pre></section>")
	writePageEnd(&b)
	_, _ = w.Write([]byte(b.String()))
}

func (s *server) handleToolJaeger(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/debug/obs/tool/jaeger" {
		http.NotFound(w, r)
		return
	}
	writeToolWrapper(w, "Jaeger", "http://127.0.0.1:16686", "Open Jaeger native")
}

func (s *server) handleToolGrafana(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/debug/obs/tool/grafana" {
		http.NotFound(w, r)
		return
	}
	writeToolWrapper(w, "Grafana", "http://127.0.0.1:13000", "Open Grafana native")
}

func (s *server) servePrompt(w http.ResponseWriter, r *http.Request, requestID, role string) {
	rec, index, err := s.loadRequestIndex(requestID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var fileName string
	for _, prompt := range index.Prompts {
		if strings.EqualFold(prompt.Role, role) {
			fileName = prompt.File
			if fileName == "" && prompt.Text != "" {
				writeText(w, prompt.Text)
				return
			}
			break
		}
	}
	if fileName == "" {
		http.NotFound(w, r)
		return
	}
	s.serveFileFromManifest(w, r, rec, index, fileName)
}

func (s *server) serveStream(w http.ResponseWriter, r *http.Request, requestID, name string) {
	switch name {
	case "raw":
		name = "stream.raw.events.jsonl"
	case "transformed":
		name = "stream.transformed.events.jsonl"
	default:
		http.Error(w, "invalid stream name", http.StatusBadRequest)
		return
	}
	rec, index, err := s.loadRequestIndex(requestID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.serveFileFromManifest(w, r, rec, index, name)
}

func (s *server) serveManifestFile(w http.ResponseWriter, r *http.Request, requestID, name string) {
	rec, index, err := s.loadRequestIndex(requestID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.serveFileFromManifest(w, r, rec, index, name)
}

func (s *server) serveFileFromManifest(w http.ResponseWriter, r *http.Request, rec requestRecord, index dump.PromptIndex, name string) {
	data, ok, err := s.readManifestBytes(rec, index, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeText(w, string(data))
}

func (s *server) readManifestFile(rec requestRecord, index dump.PromptIndex, name string) (string, error) {
	data, ok, err := s.readManifestBytes(rec, index, name)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("manifest file not found")
	}
	return string(data), nil
}

func (s *server) readManifestBytes(rec requestRecord, index dump.PromptIndex, name string) ([]byte, bool, error) {
	target, ok, err := s.manifestTargetPath(rec, index, name)
	if err != nil || !ok {
		return nil, ok, err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, false, nil
	}
	return data, true, nil
}

func (s *server) manifestTargetPath(rec requestRecord, index dump.PromptIndex, name string) (string, bool, error) {
	if !safeFileName(name) {
		return "", false, fmt.Errorf("invalid file name")
	}
	var targetRel string
	for _, file := range index.Files {
		if file.LogicalName == name || filepath.Base(file.Path) == name {
			targetRel = file.Path
			break
		}
	}
	if targetRel == "" {
		return "", false, nil
	}
	if !strings.HasPrefix(filepath.ToSlash(targetRel), strings.TrimRight(rec.Path, "/")+"/") {
		return "", false, fmt.Errorf("manifest path mismatch")
	}
	target := filepath.Join(s.root, filepath.FromSlash(targetRel))
	if !withinRoot(s.root, target) {
		return "", false, fmt.Errorf("invalid target")
	}
	return target, true, nil
}

func (s *server) loadRequestIndex(requestID string) (requestRecord, dump.PromptIndex, error) {
	records, err := s.findRecords(func(rec requestRecord) bool { return rec.RequestID == requestID })
	if err != nil || len(records) == 0 {
		return requestRecord{}, dump.PromptIndex{}, fmt.Errorf("request not found")
	}
	rec := records[len(records)-1]
	index, err := s.loadIndexForRecord(rec)
	if err != nil {
		return requestRecord{}, dump.PromptIndex{}, err
	}
	return rec, index, nil
}

func (s *server) loadIndexForRecord(rec requestRecord) (dump.PromptIndex, error) {
	indexPath := filepath.Join(s.root, filepath.FromSlash(rec.Path), "prompt.index.json")
	if !withinRoot(s.root, indexPath) {
		return dump.PromptIndex{}, fmt.Errorf("invalid index path")
	}
	stat, err := os.Stat(indexPath)
	if err != nil {
		return dump.PromptIndex{}, err
	}
	modUnixNano := stat.ModTime().UnixNano()
	size := stat.Size()
	s.cacheMu.Lock()
	if s.indexCache != nil {
		if cached, ok := s.indexCache[indexPath]; ok && cached.modUnixNano == modUnixNano && cached.size == size {
			s.cacheMu.Unlock()
			return cached.index, nil
		}
	}
	s.cacheMu.Unlock()
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		return dump.PromptIndex{}, err
	}
	var index dump.PromptIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		return dump.PromptIndex{}, err
	}
	s.cacheMu.Lock()
	if s.indexCache == nil {
		s.indexCache = map[string]cachedPromptIndex{}
	}
	s.indexCache[indexPath] = cachedPromptIndex{modUnixNano: modUnixNano, size: size, index: index}
	s.cacheMu.Unlock()
	return index, nil
}

func (s *server) findRecords(match func(requestRecord) bool) ([]requestRecord, error) {
	var out []requestRecord
	var sequence int64
	runDirs, err := filepath.Glob(filepath.Join(s.root, "runs", "*"))
	if err != nil {
		return nil, err
	}
	for _, runDir := range runDirs {
		path := filepath.Join(runDir, "requests.jsonl")
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			sequence++
			var rec requestRecord
			if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
				continue
			}
			rec.sequence = sequence
			if match(rec) {
				out = append(out, rec)
			}
		}
		if err := scanner.Err(); err != nil {
			file.Close()
			return nil, err
		}
		file.Close()
	}
	return out, nil
}

func (s *server) filterRecordsWithPrompts(records []requestRecord) []requestRecord {
	filtered := make([]requestRecord, 0, len(records))
	for _, rec := range records {
		if s.recordHasPrompts(rec) {
			filtered = append(filtered, rec)
		}
	}
	return filtered
}

func (s *server) recordHasPrompts(rec requestRecord) bool {
	index, err := s.loadIndexForRecord(rec)
	return err == nil && len(index.Prompts) > 0
}

func (s *server) buildPortalRecords(records []requestRecord) []portalRecord {
	out := make([]portalRecord, 0, len(records))
	for _, rec := range records {
		index, err := s.loadIndexForRecord(rec)
		if err != nil {
			continue
		}
		out = append(out, portalRecord{
			Record: rec,
			Index:  index,
			Kind:   s.portalRecordKind(rec, index),
		})
	}
	return out
}

func (s *server) portalRecordKind(rec requestRecord, index dump.PromptIndex) string {
	if len(index.Prompts) > 0 {
		return "prompt"
	}
	if s.isNonLLMNoise(rec, index) {
		return "noise"
	}
	return "error"
}

func filterPortalRecordsByKind(records []portalRecord, kind string) []portalRecord {
	filtered := make([]portalRecord, 0, len(records))
	for _, rec := range records {
		if rec.Kind == kind {
			filtered = append(filtered, rec)
		}
	}
	return filtered
}

func filterPortalRecordsByKinds(records []portalRecord, kinds ...string) []portalRecord {
	allowed := map[string]bool{}
	for _, kind := range kinds {
		allowed[kind] = true
	}
	filtered := make([]portalRecord, 0, len(records))
	for _, rec := range records {
		if allowed[rec.Kind] {
			filtered = append(filtered, rec)
		}
	}
	return filtered
}

func selectPortalRecordsForView(view string, prompts, errors, allLLM, noise []portalRecord) ([]portalRecord, string, string, string) {
	switch view {
	case "errors":
		return errors, "LLM API Errors", "LLM/API requests with JSON bodies but no extracted prompts.", "No LLM/API error sessions captured."
	case "all":
		return allLLM, "All LLM/API", "Prompt sessions plus LLM/API requests that produced no extracted prompts.", "No LLM/API sessions captured."
	case "noise":
		return noise, "Filtered Noise", "Non-LLM, browser resource, or empty-body traffic filtered from the default Portal list.", "No filtered noise captured."
	default:
		return prompts, "Prompt Sessions", "Only sessions with captured prompts are shown by default.", "No prompt sessions captured yet. Generate LLM traffic, then refresh this portal."
	}
}

func filterPortalRecords(records []portalRecord, query portalQuery) []portalRecord {
	filtered := make([]portalRecord, 0, len(records))
	q := strings.ToLower(query.Q)
	run := strings.ToLower(query.Run)
	role := strings.ToLower(query.Role)
	for _, item := range records {
		rec := item.Record
		if run != "" && strings.ToLower(rec.RunID) != run {
			continue
		}
		if role != "" && !portalRecordHasRole(item, role) {
			continue
		}
		if q != "" && !portalRecordMatchesQuery(item, q) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func portalRecordHasRole(item portalRecord, role string) bool {
	for _, prompt := range item.Index.Prompts {
		if strings.EqualFold(prompt.Role, role) {
			return true
		}
	}
	return false
}

func portalRecordMatchesQuery(item portalRecord, q string) bool {
	rec := item.Record
	values := []string{rec.RequestID, rec.TraceID, rec.RunID, rec.ObsRef, item.Kind}
	for _, prompt := range item.Index.Prompts {
		values = append(values, prompt.Role, prompt.Text, prompt.File)
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), q) {
			return true
		}
	}
	return false
}

func sortPortalRecords(records []portalRecord, direction string) {
	sort.SliceStable(records, func(i, j int) bool {
		cmp := compareRequestRecordsNewestFirst(records[i].Record, records[j].Record)
		if direction == "oldest" {
			return cmp > 0
		}
		return cmp < 0
	})
}

func (s *server) isNonLLMNoise(rec requestRecord, index dump.PromptIndex) bool {
	if len(index.Prompts) > 0 {
		return false
	}
	bodyBytes, hasBody := manifestFileBytes(index, "ingress.request.body.raw")
	if hasBody && bodyBytes > 0 {
		return false
	}
	if s.looksLikeBrowserResourceRequest(rec, index) {
		return true
	}
	return hasBody && bodyBytes == 0
}

func (s *server) looksLikeBrowserResourceRequest(rec requestRecord, index dump.PromptIndex) bool {
	headers, ok := s.readHeaderArtifact(rec, index)
	if !ok {
		return false
	}
	if headerHasPrefix(headers, "Sec-Fetch-Dest", "image") {
		return true
	}
	for _, value := range headers["Accept"] {
		if strings.Contains(strings.ToLower(value), "image/") {
			return true
		}
	}
	return false
}

func (s *server) readHeaderArtifact(rec requestRecord, index dump.PromptIndex) (map[string][]string, bool) {
	path, ok, err := s.manifestTargetPath(rec, index, "ingress.request.headers.json")
	if err != nil || !ok {
		return nil, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var headers map[string][]string
	if err := json.Unmarshal(raw, &headers); err != nil {
		return nil, false
	}
	return headers, true
}

func headerHasPrefix(headers map[string][]string, key, prefix string) bool {
	prefix = strings.ToLower(prefix)
	for headerKey, values := range headers {
		if !strings.EqualFold(headerKey, key) {
			continue
		}
		for _, value := range values {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), prefix) {
				return true
			}
		}
	}
	return false
}

func (s *server) buildPromptSummaries(records []requestRecord) []requestPromptSummary {
	summaries := make([]requestPromptSummary, 0, len(records))
	for _, rec := range records {
		index, err := s.loadIndexForRecord(rec)
		if err != nil || len(index.Prompts) == 0 {
			continue
		}
		summaries = append(summaries, s.promptSummaryFromIndex(rec, index))
	}
	return summaries
}

func loadCodexToolTurns(limit int) ([]codexToolTurn, string, error) {
	if limit <= 0 {
		limit = defaultCodexToolTurnLimit
	}
	root := codexSessionsRoot()
	files, err := listCodexRolloutFiles(root)
	if err != nil {
		return nil, root, err
	}
	files = sortFilesNewestFirst(files)
	turns := make([]codexToolTurn, 0, limit)
	for _, path := range files {
		fileTurns, err := parseCodexRolloutToolTurns(path)
		if err != nil {
			continue
		}
		for i := len(fileTurns) - 1; i >= 0 && len(turns) < limit; i-- {
			if len(fileTurns[i].Calls) == 0 {
				continue
			}
			turns = append(turns, fileTurns[i])
		}
		if len(turns) >= limit {
			break
		}
	}
	return turns, root, nil
}

func codexSessionsRoot() string {
	if root := strings.TrimSpace(os.Getenv("CODE_AGENT_LENS_CODEX_SESSIONS_ROOT")); root != "" {
		return root
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".codex", "sessions")
	}
	return filepath.Join(".codex", "sessions")
}

func listCodexRolloutFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info == nil || info.IsDir() {
			return nil
		}
		if strings.HasPrefix(filepath.Base(path), "rollout-") && strings.HasSuffix(path, ".jsonl") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil && os.IsNotExist(err) {
		return nil, nil
	}
	return files, err
}

func sortFilesNewestFirst(paths []string) []string {
	sort.SliceStable(paths, func(i, j int) bool {
		left, leftErr := os.Stat(paths[i])
		right, rightErr := os.Stat(paths[j])
		if leftErr != nil || rightErr != nil {
			return paths[i] > paths[j]
		}
		return left.ModTime().After(right.ModTime())
	})
	return paths
}

func parseCodexRolloutToolTurns(path string) ([]codexToolTurn, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var turns []codexToolTurn
	current := -1
	callIndex := map[string][2]int{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var event struct {
			Timestamp string          `json:"timestamp"`
			Type      string          `json:"type"`
			Payload   json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		switch event.Type {
		case "event_msg":
			var payload struct {
				Type    string `json:"type"`
				TurnID  string `json:"turn_id"`
				TraceID string `json:"trace_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.Type != "task_started" {
				continue
			}
			turns = append(turns, codexToolTurn{
				TurnID:      payload.TurnID,
				TraceID:     payload.TraceID,
				StartedAt:   event.Timestamp,
				SessionFile: filepath.Base(path),
			})
			current = len(turns) - 1
		case "response_item":
			var kind struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(event.Payload, &kind); err != nil {
				continue
			}
			switch kind.Type {
			case "function_call":
				if current < 0 {
					continue
				}
				var payload struct {
					Name      string `json:"name"`
					CallID    string `json:"call_id"`
					Arguments string `json:"arguments"`
				}
				if err := json.Unmarshal(event.Payload, &payload); err != nil {
					continue
				}
				call := codexToolCall{
					Index:       len(turns[current].Calls) + 1,
					Timestamp:   event.Timestamp,
					Name:        payload.Name,
					CallID:      payload.CallID,
					Arguments:   boundedUTF8(payload.Arguments, maxCodexToolCallSnippet),
					OutputState: "pending",
				}
				turns[current].Calls = append(turns[current].Calls, call)
				callIndex[payload.CallID] = [2]int{current, len(turns[current].Calls) - 1}
			case "function_call_output":
				var payload struct {
					CallID string `json:"call_id"`
					Output string `json:"output"`
				}
				if err := json.Unmarshal(event.Payload, &payload); err != nil {
					continue
				}
				pos, ok := callIndex[payload.CallID]
				if !ok {
					continue
				}
				call := &turns[pos[0]].Calls[pos[1]]
				call.Output = boundedUTF8(firstNonEmptyLine(payload.Output), maxCodexToolCallSnippet)
				call.OutputTime = event.Timestamp
				call.HasOutput = true
				call.OutputState = "completed"
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return turns, nil
}

func firstNonEmptyLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func (s *server) promptSummaryFromIndex(rec requestRecord, index dump.PromptIndex) requestPromptSummary {
	summary := requestPromptSummary{
		RequestID:   rec.RequestID,
		TraceID:     rec.TraceID,
		RunID:       rec.RunID,
		PromptCount: len(index.Prompts),
	}
	for _, prompt := range index.Prompts {
		role := promptRoleSummary{Role: prompt.Role, File: prompt.File}
		if prompt.File != "" {
			role.Bytes = findManifestFileBytes(index, prompt.File)
		}
		if role.Bytes == 0 && prompt.Text != "" {
			role.Bytes = len([]byte(prompt.Text))
		}
		summary.TotalBytes += role.Bytes
		summary.Roles = append(summary.Roles, role)
	}
	return summary
}

func (s *server) hydratePromptSummarySnippets(summaries []requestPromptSummary) {
	for i := range summaries {
		rec, index, err := s.loadRequestIndex(summaries[i].RequestID)
		if err != nil {
			continue
		}
		for roleIndex, prompt := range index.Prompts {
			if roleIndex >= len(summaries[i].Roles) || summaries[i].Roles[roleIndex].Snippet != "" {
				continue
			}
			summaries[i].Roles[roleIndex].Snippet = s.promptSnippet(rec, index, prompt)
		}
	}
}

func (s *server) promptSnippet(rec requestRecord, index dump.PromptIndex, prompt dump.PromptRecord) string {
	if prompt.Text != "" {
		return boundedUTF8(prompt.Text, maxPromptSnippetBytes)
	}
	if prompt.File == "" {
		return ""
	}
	data, ok, err := s.readManifestPreview(rec, index, prompt.File, maxPromptSnippetBytes)
	if err != nil {
		return "unable to read prompt file: " + err.Error()
	}
	if !ok {
		return "prompt file is missing from manifest"
	}
	return boundedUTF8(string(data), maxPromptSnippetBytes)
}

func (s *server) readManifestPreview(rec requestRecord, index dump.PromptIndex, name string, limit int) ([]byte, bool, error) {
	path, ok, err := s.manifestTargetPath(rec, index, name)
	if err != nil || !ok {
		return nil, ok, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false, nil
	}
	defer file.Close()
	limited := io.LimitReader(file, int64(limit+utf8.UTFMax))
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func (s *server) writeOverview(w http.ResponseWriter, records []requestRecord, opts pageOptions) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var b strings.Builder
	if opts.Title == "" {
		opts.Title = "CodeAgentLens Debug Viewer"
	}
	if opts.Active == "" {
		opts.Active = "debug"
	}
	if len(opts.Breadcrumb) == 0 {
		opts.Breadcrumb = []breadcrumbItem{
			{Label: "Portal", Href: "/debug/obs"},
			{Label: "Debug Viewer", Href: "/debug/obs"},
		}
	}
	writePageStart(&b, opts)
	b.WriteString("<section class=\"hero\"><p class=\"eyebrow\">Debug Viewer</p><h1>CodeAgentLens Debug Viewer</h1><p><a href=\"/debug/obs\">Portal</a> · <a href=\"/debug/obs/prompts\">all prompts</a></p></section>")
	for _, rec := range records {
		index, _ := s.loadIndexForRecord(rec)
		b.WriteString("<section class=\"request-card\">")
		b.WriteString("<p>request_id: <a href=\"/debug/obs/request/" + html.EscapeString(rec.RequestID) + "\">" + html.EscapeString(rec.RequestID) + "</a></p>")
		b.WriteString("<p>trace_id: " + html.EscapeString(rec.TraceID) + "</p>")
		b.WriteString("<p>obs_ref: " + html.EscapeString(rec.ObsRef) + "</p>")
		b.WriteString("<p><a href=\"/debug/obs/request/" + html.EscapeString(rec.RequestID) + "/prompts\">view all prompts for this request</a></p>")
		for _, prompt := range index.Prompts {
			b.WriteString("<p>" + html.EscapeString(prompt.Role) + " prompt")
			if prompt.File != "" {
				b.WriteString(": <a href=\"/debug/obs/request/" + html.EscapeString(rec.RequestID) + "/prompt/" + html.EscapeString(prompt.Role) + "\">view</a>")
			}
			if prompt.Text != "" {
				b.WriteString(": " + html.EscapeString(prompt.Text))
			}
			b.WriteString("</p>")
		}
		for _, file := range index.Files {
			label := file.LogicalName
			if strings.Contains(label, "upstream") && strings.Contains(label, "body") {
				label = "raw upstream body"
			}
			b.WriteString("<p>" + html.EscapeString(label) + ": <a href=\"/debug/obs/request/" + html.EscapeString(rec.RequestID) + "/artifact/" + html.EscapeString(file.LogicalName) + "\">view</a> · <a href=\"/debug/obs/request/" + html.EscapeString(rec.RequestID) + "/body/" + html.EscapeString(file.LogicalName) + "\" " + newTabRelAttrs + ">raw</a></p>")
		}
		b.WriteString("</section>")
	}
	writePageEnd(&b)
	_, _ = w.Write([]byte(b.String()))
}

func sortRecordsNewestFirst(records []requestRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		return compareRequestRecordsNewestFirst(records[i], records[j]) < 0
	})
}

func compareRequestRecordsNewestFirst(left, right requestRecord) int {
	leftTime, leftOK := recordTimestamp(left)
	rightTime, rightOK := recordTimestamp(right)
	if leftOK && rightOK && !leftTime.Equal(rightTime) {
		if leftTime.After(rightTime) {
			return -1
		}
		return 1
	}
	if leftOK != rightOK {
		if leftOK {
			return -1
		}
		return 1
	}
	if left.RunID != right.RunID {
		if left.RunID > right.RunID {
			return -1
		}
		return 1
	}
	if left.sequence != right.sequence {
		if left.sequence > right.sequence {
			return -1
		}
		return 1
	}
	if left.RequestID > right.RequestID {
		return -1
	}
	if left.RequestID < right.RequestID {
		return 1
	}
	return 0
}

func recordTimestamp(rec requestRecord) (time.Time, bool) {
	if rec.Timestamp == "" {
		return time.Time{}, false
	}
	value, err := time.Parse(time.RFC3339Nano, rec.Timestamp)
	if err != nil {
		return time.Time{}, false
	}
	return value, true
}

func formatRecordTime(rec requestRecord) string {
	value, ok := recordTimestamp(rec)
	if !ok {
		if rec.RunID != "" {
			return rec.RunID
		}
		return "unknown"
	}
	return value.Local().Format("2006-01-02 15:04:05")
}

func limitRecords(records []requestRecord, limit int) []requestRecord {
	if len(records) <= limit {
		return records
	}
	return records[:limit]
}

func countRuns(records []requestRecord) int {
	seen := map[string]bool{}
	for _, rec := range records {
		if rec.RunID != "" {
			seen[rec.RunID] = true
		}
	}
	return len(seen)
}

func parsePromptExplorerQuery(values url.Values) promptExplorerQuery {
	limit, err := strconv.Atoi(values.Get("limit"))
	if err != nil || limit <= 0 {
		limit = defaultPromptExplorerLimit
	}
	if limit > maxPromptExplorerLimit {
		limit = maxPromptExplorerLimit
	}
	offset, err := strconv.Atoi(values.Get("offset"))
	if err != nil || offset < 0 {
		offset = 0
	}
	return promptExplorerQuery{
		Q:      strings.TrimSpace(values.Get("q")),
		Role:   strings.TrimSpace(values.Get("role")),
		Run:    strings.TrimSpace(values.Get("run")),
		Limit:  limit,
		Offset: offset,
	}
}

func parsePortalQuery(values url.Values) portalQuery {
	limit, err := strconv.Atoi(values.Get("limit"))
	if err != nil || limit <= 0 {
		limit = defaultPromptExplorerLimit
	}
	if limit > maxPromptExplorerLimit {
		limit = maxPromptExplorerLimit
	}
	offset, err := strconv.Atoi(values.Get("offset"))
	if err != nil || offset < 0 {
		offset = 0
	}
	view := strings.ToLower(strings.TrimSpace(values.Get("view")))
	if view != "errors" && view != "all" && view != "noise" {
		view = ""
	}
	sortDirection := strings.ToLower(strings.TrimSpace(values.Get("sort")))
	if sortDirection != "oldest" {
		sortDirection = "newest"
	}
	return portalQuery{
		Limit:  limit,
		Offset: offset,
		View:   view,
		Sort:   sortDirection,
		Q:      strings.TrimSpace(values.Get("q")),
		Run:    strings.TrimSpace(values.Get("run")),
		Role:   strings.TrimSpace(values.Get("role")),
	}
}

func filterPromptSummaries(summaries []requestPromptSummary, query promptExplorerQuery) []requestPromptSummary {
	filtered := make([]requestPromptSummary, 0, len(summaries))
	q := strings.ToLower(query.Q)
	roleFilter := strings.ToLower(query.Role)
	runFilter := strings.ToLower(query.Run)
	for _, summary := range summaries {
		if runFilter != "" && strings.ToLower(summary.RunID) != runFilter {
			continue
		}
		if roleFilter != "" && !summaryHasRole(summary, roleFilter) {
			continue
		}
		if q != "" && !summaryMatchesQuery(summary, q) {
			continue
		}
		filtered = append(filtered, summary)
	}
	return filtered
}

func summaryHasRole(summary requestPromptSummary, role string) bool {
	for _, item := range summary.Roles {
		if strings.EqualFold(item.Role, role) {
			return true
		}
	}
	return false
}

func summaryMatchesQuery(summary requestPromptSummary, q string) bool {
	values := []string{summary.RequestID, summary.TraceID, summary.RunID}
	for _, item := range summary.Roles {
		values = append(values, item.Role, item.Snippet)
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), q) {
			return true
		}
	}
	return false
}

func paginatePromptSummaries(summaries []requestPromptSummary, query promptExplorerQuery) []requestPromptSummary {
	if query.Offset >= len(summaries) {
		return nil
	}
	end := query.Offset + query.Limit
	if end > len(summaries) {
		end = len(summaries)
	}
	return summaries[query.Offset:end]
}

func paginateRecords(records []requestRecord, limit, offset int) []requestRecord {
	if offset >= len(records) {
		return nil
	}
	end := offset + limit
	if end > len(records) {
		end = len(records)
	}
	return records[offset:end]
}

func paginatePortalRecords(records []portalRecord, limit, offset int) []portalRecord {
	if offset >= len(records) {
		return nil
	}
	end := offset + limit
	if end > len(records) {
		end = len(records)
	}
	return records[offset:end]
}

func findManifestFileBytes(index dump.PromptIndex, name string) int {
	bytes, _ := manifestFileBytes(index, name)
	return bytes
}

func manifestFileBytes(index dump.PromptIndex, name string) (int, bool) {
	for _, file := range index.Files {
		if file.LogicalName == name || filepath.Base(file.Path) == name {
			return file.Bytes, true
		}
	}
	return 0, false
}

func boundedUTF8(value string, maxBytes int) string {
	if len([]byte(value)) <= maxBytes {
		return value
	}
	data := []byte(value)
	cut := maxBytes - len("...")
	if cut < 1 {
		cut = maxBytes
	}
	for cut > 0 && !utf8.Valid(data[:cut]) {
		cut--
	}
	if cut <= 0 {
		return ""
	}
	return string(data[:cut]) + "..."
}

func writePageStart(b *strings.Builder, opts pageOptions) {
	if opts.Title == "" {
		opts.Title = overviewTitle
	}
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><title>")
	b.WriteString(html.EscapeString(opts.Title))
	b.WriteString("</title><style>")
	b.WriteString(pageCSS)
	b.WriteString("</style></head><body><div class=\"loading-indicator\" role=\"status\" aria-live=\"polite\"><span>Loading destination</span></div><div class=\"app-shell\"><aside class=\"sidebar\"><div class=\"brand\"><span class=\"brand-mark\"></span><strong>CodeAgentLens Observability</strong></div><nav class=\"primary-nav\">")
	writeNavLink(b, "Portal", "/debug/obs", opts.Active == "portal")
	writeNavLink(b, "Prompts", "/debug/obs/prompts", opts.Active == "prompts")
	writeNavLink(b, "Debug Viewer", "/debug/obs", opts.Active == "debug")
	b.WriteString("</nav><div class=\"quick-nav\"><p>Quick links</p>")
	writeInlineLink(b, "Health", "/health", true)
	writeInlineLink(b, "Web UI", "/ui/", true)
	writeInlineLink(b, "Grafana", "/debug/obs/tool/grafana", false)
	writeInlineLink(b, "Jaeger", "/debug/obs/tool/jaeger", false)
	b.WriteString("</div></aside><main class=\"shell\"><div class=\"topbar\">")
	writeBreadcrumbs(b, opts.Breadcrumb)
	b.WriteString("</div>")
}

func writePageEnd(b *strings.Builder) {
	b.WriteString("</main></div><script>(function(){var d=document;function sameOrigin(href){try{var u=new URL(href,location.href);return u.origin===location.origin;}catch(_){return false;}}d.addEventListener('click',function(e){var a=e.target.closest&&e.target.closest('a[href]');if(!a||a.target||a.hasAttribute('download'))return;var href=a.getAttribute('href')||'';if(href===''||href.charAt(0)==='#'||!sameOrigin(href))return;d.body.classList.add('is-loading');},{capture:true});function loadPreview(card){if(!card||card.dataset.loaded||card.dataset.loading)return;var url=card.dataset.previewUrl;if(!url)return;card.dataset.loading='true';var body=card.querySelector('.hover-card-body');if(body){body.innerHTML='<p class=\"muted\">Loading prompt preview...</p>';}fetch(url,{headers:{'X-Requested-With':'code-agent-lens-debug-viewer'}}).then(function(resp){if(!resp.ok)throw new Error('HTTP '+resp.status);return resp.text();}).then(function(html){if(body){body.innerHTML=html;}card.dataset.loaded='true';}).catch(function(err){if(body){body.innerHTML='<p class=\"muted preview-error\">Unable to load prompt preview: '+String(err.message||err)+'</p>';}}).finally(function(){delete card.dataset.loading;});}function previewFromEvent(e){var cell=e.target.closest&&e.target.closest('.trace-cell');loadPreview(cell&&cell.querySelector('.hover-card'));}d.addEventListener('pointerover',previewFromEvent);d.addEventListener('focusin',previewFromEvent);window.addEventListener('pageshow',function(){d.body.classList.remove('is-loading');});})();</script></body></html>")
}

type toolCard struct {
	Title  string
	Desc   string
	Href   string
	Code   string
	NewTab bool
}

func writeToolCard(b *strings.Builder, item toolCard) {
	b.WriteString("<a class=\"tool-card\" href=\"" + html.EscapeString(item.Href) + "\"")
	if item.NewTab {
		b.WriteString(" " + newTabRelAttrs)
	}
	b.WriteString("><strong>" + html.EscapeString(item.Title) + "</strong><span>" + html.EscapeString(item.Desc) + "</span><code>" + html.EscapeString(item.Code) + "</code></a>")
}

func writeStatusTile(b *strings.Builder, label, value, href string, newTab bool) {
	b.WriteString("<a class=\"status-tile\" href=\"" + html.EscapeString(href) + "\"")
	if newTab {
		b.WriteString(" " + newTabRelAttrs)
	}
	b.WriteString("><span>" + html.EscapeString(label) + "</span><strong>" + html.EscapeString(value) + "</strong></a>")
}

func writeNavLink(b *strings.Builder, label, href string, active bool) {
	class := "nav-link"
	if active {
		class += " active"
	}
	b.WriteString("<a class=\"" + class + "\" href=\"" + html.EscapeString(href) + "\">" + html.EscapeString(label) + "</a>")
}

func writeInlineLink(b *strings.Builder, label, href string, newTab bool) {
	b.WriteString("<a href=\"" + html.EscapeString(href) + "\"")
	if newTab {
		b.WriteString(" " + newTabRelAttrs)
	}
	b.WriteString(">" + html.EscapeString(label) + "</a>")
}

func writeBreadcrumbs(b *strings.Builder, items []breadcrumbItem) {
	if len(items) == 0 {
		items = []breadcrumbItem{{Label: "Portal", Href: "/debug/obs"}}
	}
	b.WriteString("<nav class=\"breadcrumbs\" aria-label=\"Breadcrumb\">")
	for i, item := range items {
		if i > 0 {
			b.WriteString(" <span>/</span> ")
		}
		if item.Href != "" {
			b.WriteString("<a href=\"" + html.EscapeString(item.Href) + "\">" + html.EscapeString(item.Label) + "</a>")
		} else {
			b.WriteString("<span>" + html.EscapeString(item.Label) + "</span>")
		}
	}
	b.WriteString("</nav>")
}

func writeRoleBadges(b *strings.Builder, prompts []dump.PromptRecord) {
	if len(prompts) == 0 {
		b.WriteString("<span class=\"muted\">none</span>")
		return
	}
	for _, prompt := range prompts {
		b.WriteString("<span class=\"role-badge\">" + html.EscapeString(prompt.Role) + "</span>")
	}
}

func writePromptTraceList(b *strings.Builder, summaries []requestPromptSummary) {
	b.WriteString("<div class=\"trace-list table-wrap\"><table class=\"obs-table prompt-trace-table\"><thead><tr><th>Trace ID</th><th>Request ID</th><th>Run</th><th>Roles</th><th>Prompts</th><th>Bytes</th><th>Actions</th></tr></thead><tbody>")
	for _, summary := range summaries {
		b.WriteString("<tr class=\"trace-row\"><td class=\"trace-cell\">")
		b.WriteString("<a class=\"trace-primary\" href=\"/debug/obs/request/" + html.EscapeString(summary.RequestID) + "/prompts\"><code>" + html.EscapeString(summary.TraceID) + "</code></a>")
		writePromptHoverCardShell(b, summary)
		b.WriteString("</td><td><a href=\"/debug/obs/request/" + html.EscapeString(summary.RequestID) + "\"><code>" + html.EscapeString(summary.RequestID) + "</code></a></td>")
		b.WriteString("<td><code>" + html.EscapeString(summary.RunID) + "</code></td><td>")
		for _, role := range summary.Roles {
			b.WriteString("<span class=\"role-badge\">" + html.EscapeString(role.Role) + "</span>")
		}
		b.WriteString("</td><td>" + strconv.Itoa(summary.PromptCount) + "</td><td>" + strconv.Itoa(summary.TotalBytes) + "</td><td class=\"row-actions\">")
		b.WriteString("<a href=\"/debug/obs/request/" + html.EscapeString(summary.RequestID) + "/prompts\">details</a>")
		b.WriteString("<a href=\"/debug/obs/request/" + html.EscapeString(summary.RequestID) + "\">request</a>")
		b.WriteString("<a href=\"/debug/obs/trace/" + html.EscapeString(summary.TraceID) + "\">trace</a>")
		b.WriteString("</td></tr>")
	}
	b.WriteString("</tbody></table></div>")
}

func writePromptHoverCardShell(b *strings.Builder, summary requestPromptSummary) {
	b.WriteString("<aside class=\"hover-card\" data-preview-url=\"/debug/obs/request/" + html.EscapeString(summary.RequestID) + "/prompt-preview\" aria-label=\"Prompt preview for trace " + html.EscapeString(summary.TraceID) + "\">")
	b.WriteString("<div class=\"hover-card-body hover-card-placeholder\"><div class=\"hover-card-head\"><strong>" + html.EscapeString(summary.TraceID) + "</strong><span>" + strconv.Itoa(summary.PromptCount) + " prompts · " + strconv.Itoa(summary.TotalBytes) + " bytes</span></div>")
	b.WriteString("<p class=\"muted\">Hover preview loads on demand. Click the trace id for full prompt details.</p></div>")
	b.WriteString("</aside>")
}

func writePromptHoverCardBody(b *strings.Builder, summary requestPromptSummary) {
	b.WriteString("<div class=\"hover-card-head\"><strong>" + html.EscapeString(summary.TraceID) + "</strong><span>" + strconv.Itoa(summary.PromptCount) + " prompts · " + strconv.Itoa(summary.TotalBytes) + " bytes</span></div>")
	b.WriteString("<p class=\"muted\">request: <code>" + html.EscapeString(summary.RequestID) + "</code> · run: <code>" + html.EscapeString(summary.RunID) + "</code></p>")
	for _, role := range summary.Roles {
		b.WriteString("<section class=\"snippet-block compact-snippet\"><div><span class=\"role-badge\">" + html.EscapeString(role.Role) + "</span>")
		if role.File != "" {
			b.WriteString("<code>" + html.EscapeString(role.File) + "</code>")
		}
		b.WriteString("</div><pre>")
		if role.Snippet == "" {
			b.WriteString("No inline preview. Open details to inspect the full prompt.")
		} else {
			b.WriteString(html.EscapeString(role.Snippet))
		}
		b.WriteString("</pre></section>")
	}
}

func writePromptSummary(b *strings.Builder, summary requestPromptSummary) {
	b.WriteString("<article class=\"request-card prompt-summary\"><header><h3><a href=\"/debug/obs/request/" + html.EscapeString(summary.RequestID) + "/prompts\">" + html.EscapeString(summary.RequestID) + "</a></h3>")
	b.WriteString("<p class=\"muted\">run: <code>" + html.EscapeString(summary.RunID) + "</code> · trace: <a href=\"/debug/obs/trace/" + html.EscapeString(summary.TraceID) + "\"><code>" + html.EscapeString(summary.TraceID) + "</code></a> · prompts: " + strconv.Itoa(summary.PromptCount) + " · bytes: " + strconv.Itoa(summary.TotalBytes) + "</p></header>")
	b.WriteString("<div class=\"role-row\">")
	for _, role := range summary.Roles {
		b.WriteString("<span class=\"role-badge\">" + html.EscapeString(role.Role) + "</span>")
	}
	b.WriteString("</div>")
	for _, role := range summary.Roles {
		b.WriteString("<section class=\"snippet-block\"><div><span class=\"role-badge\">" + html.EscapeString(role.Role) + "</span>")
		if role.File != "" {
			b.WriteString("<code>" + html.EscapeString(role.File) + "</code>")
		}
		b.WriteString("</div><pre>" + html.EscapeString(role.Snippet) + "</pre></section>")
	}
	b.WriteString("<p class=\"row-actions\"><a href=\"/debug/obs/request/" + html.EscapeString(summary.RequestID) + "\">Request details</a><a href=\"/debug/obs/request/" + html.EscapeString(summary.RequestID) + "/prompts\">Prompt details</a><a href=\"/debug/obs/trace/" + html.EscapeString(summary.TraceID) + "\">Trace details</a></p>")
	b.WriteString("</article>")
}

func writeCodexToolTurns(b *strings.Builder, turns []codexToolTurn) {
	b.WriteString("<section class=\"panel\"><div class=\"section-heading\"><p class=\"eyebrow\">Recent Turns</p><h2>Tool Calls</h2></div>")
	b.WriteString("<p class=\"muted\">Showing " + strconv.Itoa(len(turns)) + " recent turns with tool activity. Arguments and outputs are shortened for safe scanning.</p></section>")
	for _, turn := range turns {
		b.WriteString("<article class=\"request-card tool-chain-turn\"><header><h2>Turn <code>" + html.EscapeString(turn.TurnID) + "</code></h2>")
		b.WriteString("<p class=\"muted\">started: <code>" + html.EscapeString(turn.StartedAt) + "</code> · file: <code>" + html.EscapeString(turn.SessionFile) + "</code></p>")
		if turn.TraceID != "" {
			jaegerURL := "http://127.0.0.1:16686/trace/" + url.PathEscape(turn.TraceID)
			b.WriteString("<p class=\"row-actions\"><a href=\"" + html.EscapeString(jaegerURL) + "\" " + newTabRelAttrs + ">Open Jaeger trace</a><code>" + html.EscapeString(turn.TraceID) + "</code></p>")
		}
		b.WriteString("</header><div class=\"tool-chain-list\">")
		for _, call := range turn.Calls {
			b.WriteString("<section class=\"snippet-block tool-call-node\"><div><span class=\"role-badge\">#" + strconv.Itoa(call.Index) + "</span>")
			b.WriteString("<strong>" + html.EscapeString(call.Name) + "</strong><code>" + html.EscapeString(call.CallID) + "</code>")
			b.WriteString("<span class=\"muted\">" + html.EscapeString(call.Timestamp) + "</span></div>")
			if call.Arguments != "" {
				b.WriteString("<p class=\"muted\">arguments</p><pre>" + html.EscapeString(call.Arguments) + "</pre>")
			}
			b.WriteString("<p class=\"muted\">output: <span class=\"role-badge\">" + html.EscapeString(call.OutputState) + "</span>")
			if call.OutputTime != "" {
				b.WriteString(" <code>" + html.EscapeString(call.OutputTime) + "</code>")
			}
			b.WriteString("</p>")
			if call.HasOutput {
				b.WriteString("<pre>" + html.EscapeString(call.Output) + "</pre>")
			}
			b.WriteString("</section>")
		}
		b.WriteString("</div></article>")
	}
}

func writeInput(b *strings.Builder, name, label, value string) {
	b.WriteString("<label>" + html.EscapeString(label) + "<input name=\"" + html.EscapeString(name) + "\" value=\"" + html.EscapeString(value) + "\"></label>")
}

func writePagination(b *strings.Builder, query promptExplorerQuery, total int) {
	if total <= query.Limit {
		return
	}
	b.WriteString("<nav class=\"pagination\" aria-label=\"Prompt pagination\">")
	if query.Offset > 0 {
		prev := query
		prev.Offset -= query.Limit
		if prev.Offset < 0 {
			prev.Offset = 0
		}
		b.WriteString("<a href=\"/debug/obs/prompts?" + html.EscapeString(prev.queryString()) + "\">Previous</a>")
	}
	if query.Offset+query.Limit < total {
		next := query
		next.Offset += query.Limit
		b.WriteString("<a href=\"/debug/obs/prompts?" + html.EscapeString(next.queryString()) + "\">Next</a>")
	}
	b.WriteString("</nav>")
}

func writePortalPagination(b *strings.Builder, query portalQuery, total int) {
	if total <= query.Limit {
		return
	}
	b.WriteString("<nav class=\"pagination\" aria-label=\"Session pagination\">")
	if query.Offset > 0 {
		prev := query
		prev.Offset -= query.Limit
		if prev.Offset < 0 {
			prev.Offset = 0
		}
		b.WriteString("<a href=\"/debug/obs?" + html.EscapeString(prev.queryString()) + "\">Previous</a>")
	}
	if query.Offset+query.Limit < total {
		next := query
		next.Offset += query.Limit
		b.WriteString("<a href=\"/debug/obs?" + html.EscapeString(next.queryString()) + "\">Next</a>")
	}
	b.WriteString("</nav>")
}

func writePortalFilters(b *strings.Builder, query portalQuery) {
	b.WriteString("<div class=\"filter-rail\" style=\"position:static;margin:16px 0;\"><div class=\"row-actions\">")
	for _, item := range []struct {
		label string
		view  string
	}{
		{label: "Prompt Sessions", view: ""},
		{label: "LLM API Errors", view: "errors"},
		{label: "All LLM/API", view: "all"},
		{label: "Noise", view: "noise"},
	} {
		next := query
		next.View = item.view
		next.Offset = 0
		class := ""
		if query.View == item.view {
			class = " class=\"role-badge\""
		}
		b.WriteString("<a" + class + " href=\"/debug/obs?" + html.EscapeString(next.queryString()) + "\">" + html.EscapeString(item.label) + "</a>")
	}
	b.WriteString("<a href=\"/debug/obs/prompts\">Prompt Explorer</a></div>")
	b.WriteString("<form method=\"get\" action=\"/debug/obs\" class=\"filter-form\" style=\"margin-top:14px;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));align-items:end;\">")
	if query.View != "" {
		b.WriteString("<input type=\"hidden\" name=\"view\" value=\"" + html.EscapeString(query.View) + "\">")
	}
	b.WriteString("<label>Search<input name=\"q\" value=\"" + html.EscapeString(query.Q) + "\" placeholder=\"request / trace / run\"></label>")
	b.WriteString("<label>Run<input name=\"run\" value=\"" + html.EscapeString(query.Run) + "\" placeholder=\"run id\"></label>")
	b.WriteString("<label>Role<input name=\"role\" value=\"" + html.EscapeString(query.Role) + "\" placeholder=\"system / developer / user\"></label>")
	b.WriteString("<label>Sort<select name=\"sort\">")
	for _, item := range []struct {
		value string
		label string
	}{
		{value: "newest", label: "Newest first"},
		{value: "oldest", label: "Oldest first"},
	} {
		selected := ""
		if query.Sort == item.value {
			selected = " selected"
		}
		b.WriteString("<option value=\"" + item.value + "\"" + selected + ">" + item.label + "</option>")
	}
	b.WriteString("</select></label>")
	b.WriteString("<input type=\"hidden\" name=\"limit\" value=\"" + strconv.Itoa(query.Limit) + "\">")
	b.WriteString("<button type=\"submit\">Apply filters</button><a class=\"reset-link\" href=\"/debug/obs\">Reset</a></form></div>")
}

func writePortalSessionTable(b *strings.Builder, records []portalRecord) {
	b.WriteString("<div class=\"table-wrap\"><table class=\"obs-table\"><thead><tr><th>time</th><th>request id</th><th>trace id</th><th>prompts</th><th>actions</th></tr></thead><tbody>")
	for _, item := range records {
		rec := item.Record
		index := item.Index
		b.WriteString("<tr><td>" + html.EscapeString(formatRecordTime(rec)) + "</td><td><a href=\"/debug/obs/request/" + html.EscapeString(rec.RequestID) + "\"><code>" + html.EscapeString(rec.RequestID) + "</code></a></td>")
		b.WriteString("<td><a href=\"/debug/obs/trace/" + html.EscapeString(rec.TraceID) + "\"><code>" + html.EscapeString(rec.TraceID) + "</code></a></td>")
		b.WriteString("<td>")
		if len(index.Prompts) == 0 {
			b.WriteString("<span class=\"muted\">none</span>")
		} else {
			b.WriteString(strconv.Itoa(len(index.Prompts)))
		}
		b.WriteString("</td><td class=\"row-actions\">")
		b.WriteString("<a href=\"/debug/obs/request/" + html.EscapeString(rec.RequestID) + "\">request</a>")
		if len(index.Prompts) > 0 {
			b.WriteString("<a href=\"/debug/obs/request/" + html.EscapeString(rec.RequestID) + "/prompts\">prompts</a>")
		}
		b.WriteString("<a href=\"/debug/obs/trace/" + html.EscapeString(rec.TraceID) + "\">trace</a>")
		b.WriteString("</td></tr>")
	}
	b.WriteString("</tbody></table></div>")
}

func (query promptExplorerQuery) queryString() string {
	values := url.Values{}
	if query.Q != "" {
		values.Set("q", query.Q)
	}
	if query.Role != "" {
		values.Set("role", query.Role)
	}
	if query.Run != "" {
		values.Set("run", query.Run)
	}
	values.Set("limit", strconv.Itoa(query.Limit))
	values.Set("offset", strconv.Itoa(query.Offset))
	return values.Encode()
}

func (query portalQuery) queryString() string {
	values := url.Values{}
	if query.View != "" {
		values.Set("view", query.View)
	}
	if query.Sort != "" && query.Sort != "newest" {
		values.Set("sort", query.Sort)
	}
	if query.Q != "" {
		values.Set("q", query.Q)
	}
	if query.Run != "" {
		values.Set("run", query.Run)
	}
	if query.Role != "" {
		values.Set("role", query.Role)
	}
	values.Set("limit", strconv.Itoa(query.Limit))
	values.Set("offset", strconv.Itoa(query.Offset))
	return values.Encode()
}

func writeToolWrapper(w http.ResponseWriter, title, iframeURL, nativeLabel string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var b strings.Builder
	writePageStart(&b, pageOptions{
		Title:  title,
		Active: "portal",
		Breadcrumb: []breadcrumbItem{
			{Label: "Portal", Href: "/debug/obs"},
			{Label: "Tools", Href: "/debug/obs"},
			{Label: title, Href: ""},
		},
	})
	b.WriteString("<section class=\"hero tool-hero\"><p class=\"eyebrow\">Integrated tool</p><h1>" + html.EscapeString(title) + "</h1>")
	b.WriteString("<p>Portal / Tools / " + html.EscapeString(title) + "</p>")
	b.WriteString("<p><a href=\"" + html.EscapeString(iframeURL) + "\" " + newTabRelAttrs + ">" + html.EscapeString(nativeLabel) + "</a></p></section>")
	b.WriteString("<section class=\"panel tool-frame-panel\"><p class=\"frame-policy\">If browser frame policy blocks embedding, use the native open link above. The Portal navigation remains visible here.</p>")
	b.WriteString("<iframe class=\"tool-frame\" src=\"" + html.EscapeString(iframeURL) + "\" title=\"" + html.EscapeString(title) + "\"></iframe></section>")
	writePageEnd(&b)
	_, _ = w.Write([]byte(b.String()))
}

func writeText(w http.ResponseWriter, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(text))
}

func safeID(value string) bool {
	if value == "" || strings.Contains(value, "..") || strings.ContainsAny(value, `/\:`) {
		return false
	}
	return true
}

func safeFileName(value string) bool {
	if value == "" || strings.Contains(value, "..") || filepath.IsAbs(value) || strings.Contains(value, "/") || strings.Contains(value, "\\") {
		return false
	}
	if len(value) >= 2 && value[1] == ':' {
		return false
	}
	return true
}

func withinRoot(root, target string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}
