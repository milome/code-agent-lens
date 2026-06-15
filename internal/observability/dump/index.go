package dump

type FileRecord struct {
	LogicalName string `json:"logical_name"`
	Path        string `json:"path"`
	SHA256      string `json:"sha256"`
	Bytes       int    `json:"bytes"`
}

type PromptRecord struct {
	Role string `json:"role"`
	Text string `json:"text,omitempty"`
	File string `json:"file,omitempty"`
}

type PromptIndex struct {
	RunID     string         `json:"run_id"`
	RequestID string         `json:"request_id"`
	TraceID   string         `json:"trace_id"`
	SpanID    string         `json:"span_id"`
	Prompts   []PromptRecord `json:"prompts"`
	Files     []FileRecord   `json:"files"`
	SHA256    string         `json:"sha256,omitempty"`
	Bytes     int            `json:"bytes,omitempty"`
}
