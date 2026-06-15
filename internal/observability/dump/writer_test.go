package dump

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriterDisabledDoesNotCreateRequestDirectory(t *testing.T) {
	root := t.TempDir()
	w := NewWriter(Config{Enabled: false, Root: root, RunID: "run-1"})

	req, err := w.BeginRequest("trace-1", "span-1", "request-1")
	if err != nil {
		t.Fatalf("BeginRequest disabled error = %v", err)
	}
	if req.Enabled() {
		t.Fatalf("request writer enabled, want false")
	}
	if _, err := os.Stat(filepath.Join(root, "runs")); !os.IsNotExist(err) {
		t.Fatalf("runs directory exists or unexpected stat error: %v", err)
	}
}

func TestWriterWritesFilesIndexAndRequestsJSONL(t *testing.T) {
	root := t.TempDir()
	w := NewWriter(Config{
		Enabled:              true,
		Root:                 root,
		RunID:                "run-1",
		CaptureHeaders:       "all",
		CaptureBodies:        "all",
		CaptureSecrets:       true,
		CaptureStreamEvents:  "all",
		MaxBodyBytes:         0,
		PromptExtractEnabled: true,
	})

	req, err := w.BeginRequest("trace-1", "span-1", "request-1")
	if err != nil {
		t.Fatalf("BeginRequest error = %v", err)
	}
	rec, err := req.WriteFile("ingress.request.body.raw", []byte("secret body"))
	if err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	if rec.Path != "runs/run-1/traces/trace-1/request-1/ingress.request.body.raw" {
		t.Fatalf("relative path = %q", rec.Path)
	}
	if rec.Bytes != len("secret body") || rec.SHA256 == "" {
		t.Fatalf("bad record: %+v", rec)
	}
	if got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rec.Path))); err != nil || string(got) != "secret body" {
		t.Fatalf("dump content = %q, err=%v", string(got), err)
	}

	if _, err := req.AppendJSONL("stream.raw.events.jsonl", map[string]any{"event_index": 1, "data": "x"}); err != nil {
		t.Fatalf("AppendJSONL error = %v", err)
	}
	if err := req.WritePromptIndex(PromptIndex{Prompts: []PromptRecord{{Role: "system", File: "prompt.system.txt"}}}); err != nil {
		t.Fatalf("WritePromptIndex error = %v", err)
	}

	indexPath := filepath.Join(root, "runs", "run-1", "traces", "trace-1", "request-1", "prompt.index.json")
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read prompt.index.json: %v", err)
	}
	var index PromptIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatalf("unmarshal prompt index: %v", err)
	}
	if index.RunID != "run-1" || index.RequestID != "request-1" || index.TraceID != "trace-1" || index.SpanID != "span-1" {
		t.Fatalf("index ids not populated: %+v", index)
	}
	if len(index.Files) == 0 {
		t.Fatalf("index files missing: %+v", index)
	}

	requestsPath := filepath.Join(root, "runs", "run-1", "requests.jsonl")
	requestsRaw, err := os.ReadFile(requestsPath)
	if err != nil {
		t.Fatalf("read requests.jsonl: %v", err)
	}
	if !strings.Contains(string(requestsRaw), `"request_id":"request-1"`) {
		t.Fatalf("requests.jsonl missing request id: %s", requestsRaw)
	}
}

func TestWriterRedactsSecretsWhenDisabled(t *testing.T) {
	root := t.TempDir()
	w := NewWriter(Config{Enabled: true, Root: root, RunID: "run-1", CaptureHeaders: "all", CaptureSecrets: false})
	req, err := w.BeginRequest("trace-1", "span-1", "request-1")
	if err != nil {
		t.Fatalf("BeginRequest error = %v", err)
	}
	headers := map[string][]string{
		"Authorization": {"Bearer secret"},
		"x-api-key":     {"api-secret"},
		"Other":         {"ok"},
	}
	if _, err := req.WriteJSONFile("ingress.request.headers.json", RedactHeaders(headers, false)); err != nil {
		t.Fatalf("WriteJSONFile error = %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "runs", "run-1", "traces", "trace-1", "request-1", "ingress.request.headers.json"))
	if err != nil {
		t.Fatalf("read headers: %v", err)
	}
	text := string(raw)
	if strings.Contains(text, "secret") || !strings.Contains(text, "[redacted]") || !strings.Contains(text, "ok") {
		t.Fatalf("redaction failed: %s", text)
	}
}

func TestWriterKeepsSecretsAndUnlimitedBodyWhenEnabled(t *testing.T) {
	root := t.TempDir()
	w := NewWriter(Config{Enabled: true, Root: root, RunID: "run-1", CaptureSecrets: true, MaxBodyBytes: 0})
	req, err := w.BeginRequest("trace-1", "span-1", "request-1")
	if err != nil {
		t.Fatalf("BeginRequest error = %v", err)
	}
	headers := map[string][]string{
		"Authorization": {"Bearer secret-token"},
		"x-api-key":     {"api-secret"},
	}
	if _, err := req.WriteJSONFile("ingress.request.headers.json", RedactHeaders(headers, true)); err != nil {
		t.Fatalf("WriteJSONFile error = %v", err)
	}
	body := strings.Repeat("x", 1024)
	if _, err := req.WriteFile("ingress.request.body.raw", []byte(body)); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	headersRaw, err := os.ReadFile(filepath.Join(root, "runs", "run-1", "traces", "trace-1", "request-1", "ingress.request.headers.json"))
	if err != nil {
		t.Fatalf("read headers: %v", err)
	}
	if !strings.Contains(string(headersRaw), "secret-token") || !strings.Contains(string(headersRaw), "api-secret") {
		t.Fatalf("capture secrets enabled should keep secret values: %s", headersRaw)
	}
	bodyRaw, err := os.ReadFile(filepath.Join(root, "runs", "run-1", "traces", "trace-1", "request-1", "ingress.request.body.raw"))
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(bodyRaw) != body {
		t.Fatalf("unlimited body was truncated: got=%d want=%d", len(bodyRaw), len(body))
	}
}
