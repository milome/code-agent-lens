package dump

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Writer struct {
	cfg Config
	mu  sync.Mutex
}

type RequestWriter struct {
	writer    *Writer
	enabled   bool
	runID     string
	traceID   string
	spanID    string
	requestID string
	dir       string
	relDir    string
	files     []FileRecord
}

func NewWriter(cfg Config) *Writer {
	return &Writer{cfg: cfg}
}

func (w *Writer) BeginRequest(traceID string, spanID string, requestID string) (*RequestWriter, error) {
	if w == nil || !w.cfg.Enabled {
		return &RequestWriter{enabled: false}, nil
	}
	relDir := filepath.ToSlash(filepath.Join("runs", w.cfg.RunID, "traces", traceID, requestID))
	dir := filepath.Join(w.cfg.Root, filepath.FromSlash(relDir))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	rw := &RequestWriter{
		writer:    w,
		enabled:   true,
		runID:     w.cfg.RunID,
		traceID:   traceID,
		spanID:    spanID,
		requestID: requestID,
		dir:       dir,
		relDir:    relDir,
	}
	_ = w.appendRunRequest(map[string]any{
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
		"run_id":     w.cfg.RunID,
		"trace_id":   traceID,
		"span_id":    spanID,
		"request_id": requestID,
		"obs_ref":    traceID + "/" + requestID,
		"path":       relDir,
	})
	return rw, nil
}

func (r *RequestWriter) Enabled() bool {
	return r != nil && r.enabled
}

func (r *RequestWriter) WriteFile(logicalName string, data []byte) (FileRecord, error) {
	if r == nil || !r.enabled {
		return FileRecord{}, nil
	}
	if r.writer.cfg.MaxBodyBytes > 0 && int64(len(data)) > r.writer.cfg.MaxBodyBytes {
		data = data[:r.writer.cfg.MaxBodyBytes]
	}
	filename := sanitizeLogicalName(logicalName)
	path := filepath.Join(r.dir, filename)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return FileRecord{}, err
	}
	rec := FileRecord{
		LogicalName: logicalName,
		Path:        filepath.ToSlash(filepath.Join(r.relDir, filename)),
		SHA256:      hashBytes(data),
		Bytes:       len(data),
	}
	r.files = append(r.files, rec)
	return rec, nil
}

func (r *RequestWriter) WriteJSONFile(logicalName string, value any) (FileRecord, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return FileRecord{}, err
	}
	data = append(data, '\n')
	return r.WriteFile(logicalName, data)
}

func (r *RequestWriter) AppendJSONL(logicalName string, value any) (FileRecord, error) {
	if r == nil || !r.enabled {
		return FileRecord{}, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return FileRecord{}, err
	}
	data = append(data, '\n')
	filename := sanitizeLogicalName(logicalName)
	path := filepath.Join(r.dir, filename)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return FileRecord{}, err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return FileRecord{}, err
	}
	combined, _ := os.ReadFile(path)
	rec := FileRecord{
		LogicalName: logicalName,
		Path:        filepath.ToSlash(filepath.Join(r.relDir, filename)),
		SHA256:      hashBytes(combined),
		Bytes:       len(combined),
	}
	r.upsertFileRecord(rec)
	return rec, nil
}

func (r *RequestWriter) WritePromptIndex(index PromptIndex) error {
	if r == nil || !r.enabled {
		return nil
	}
	index.RunID = r.runID
	index.RequestID = r.requestID
	index.TraceID = r.traceID
	index.SpanID = r.spanID
	index.Files = append(index.Files, r.files...)
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(r.dir, "prompt.index.json")
	return os.WriteFile(path, data, 0600)
}

func (r *RequestWriter) upsertFileRecord(rec FileRecord) {
	for i, existing := range r.files {
		if existing.LogicalName == rec.LogicalName {
			r.files[i] = rec
			return
		}
	}
	r.files = append(r.files, rec)
}

func (w *Writer) appendRunRequest(value any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Join(w.cfg.Root, "runs", w.cfg.RunID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "requests.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

func RedactHeaders(headers map[string][]string, captureSecrets bool) map[string][]string {
	out := make(map[string][]string, len(headers))
	for k, values := range headers {
		if !captureSecrets && isSecretHeader(k) {
			out[k] = []string{"[redacted]"}
			continue
		}
		out[k] = append([]string(nil), values...)
	}
	return out
}

func isSecretHeader(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	return normalized == "authorization" || normalized == "x-api-key" ||
		strings.Contains(normalized, "access-token") ||
		strings.Contains(normalized, "refresh-token")
}

func sanitizeLogicalName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	if name == "." || name == "" {
		return "dump.bin"
	}
	return name
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
