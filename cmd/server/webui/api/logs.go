package api

import (
	"net/http"
	"strconv"

	"github.com/milome/code-agent-lens/internal/logger"
)

const (
	defaultLogLimit = 200
	maxLogLimit     = 1000
)

type logsResponse struct {
	Logs        []logger.LogEntry `json:"logs"`
	Total       int               `json:"total"`
	Limit       int               `json:"limit"`
	Level       int               `json:"level"`
	IsTruncated bool              `json:"isTruncated"`
}

func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getLogs(w, r)
	case http.MethodDelete:
		logger.GetLogger().Clear()
		WriteSuccess(w, map[string]interface{}{
			"message": "Logs cleared successfully",
		})
	default:
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (h *Handler) getLogs(w http.ResponseWriter, r *http.Request) {
	level, ok := parseLogLevelParam(r.URL.Query().Get("level"))
	if !ok {
		WriteError(w, http.StatusBadRequest, "Invalid log level (must be 0-3)")
		return
	}

	limit, ok := parseLogLimitParam(r.URL.Query().Get("limit"))
	if !ok {
		WriteError(w, http.StatusBadRequest, "Invalid limit")
		return
	}

	logs := logger.GetLogger().GetLogsByLevel(logger.LogLevel(level))
	total := len(logs)
	truncated := total > limit
	if truncated {
		logs = logs[total-limit:]
	}

	WriteSuccess(w, logsResponse{
		Logs:        logs,
		Total:       total,
		Limit:       limit,
		Level:       level,
		IsTruncated: truncated,
	})
}

func parseLogLevelParam(raw string) (int, bool) {
	if raw == "" {
		return int(logger.DEBUG), true
	}
	level, err := strconv.Atoi(raw)
	if err != nil || level < int(logger.DEBUG) || level > int(logger.ERROR) {
		return 0, false
	}
	return level, true
}

func parseLogLimitParam(raw string) (int, bool) {
	if raw == "" {
		return defaultLogLimit, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, false
	}
	if limit > maxLogLimit {
		return maxLogLimit, true
	}
	return limit, true
}
