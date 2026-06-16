package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/milome/code-agent-lens/internal/config"
	"github.com/milome/code-agent-lens/internal/logger"
)

func TestHandleLogsReturnsFilteredLimitedEntries(t *testing.T) {
	log := logger.GetLogger()
	log.Clear()
	log.SetMinLevel(logger.DEBUG)
	t.Cleanup(log.Clear)

	logger.Debug("debug entry")
	logger.Info("info entry")
	logger.Warn("warn entry")
	logger.Error("error entry")

	handler := NewHandler(&config.Config{}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/logs?level=1&limit=2", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Logs []struct {
				Level   logger.LogLevel `json:"level"`
				Message string          `json:"message"`
			} `json:"logs"`
			Total       int  `json:"total"`
			Limit       int  `json:"limit"`
			Level       int  `json:"level"`
			IsTruncated bool `json:"isTruncated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !response.Success {
		t.Fatalf("expected success response")
	}
	if response.Data.Total != 3 || response.Data.Limit != 2 || response.Data.Level != 1 || !response.Data.IsTruncated {
		t.Fatalf("unexpected metadata: %+v", response.Data)
	}
	if len(response.Data.Logs) != 2 {
		t.Fatalf("expected 2 logs, got len=%d", len(response.Data.Logs))
	}
	if response.Data.Logs[0].Message != "warn entry" || response.Data.Logs[1].Message != "error entry" {
		t.Fatalf("unexpected logs: %+v", response.Data.Logs)
	}
}

func TestHandleLogsRejectsInvalidLevel(t *testing.T) {
	handler := NewHandler(&config.Config{}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/logs?level=9", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Invalid log level") {
		t.Fatalf("unexpected body=%s", rec.Body.String())
	}
}

func TestHandleLogsClearsEntries(t *testing.T) {
	log := logger.GetLogger()
	log.Clear()
	log.SetMinLevel(logger.DEBUG)
	t.Cleanup(log.Clear)

	logger.Info("entry to clear")

	handler := NewHandler(&config.Config{}, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/api/logs", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if got := logger.GetLogger().GetLogs(); len(got) != 0 {
		t.Fatalf("logs not cleared: %+v", got)
	}
}
