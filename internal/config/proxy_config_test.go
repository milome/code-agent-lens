package config

import (
	"fmt"
	"testing"
)

type fakeConfigStorage struct {
	values map[string]string
}

func (s *fakeConfigStorage) GetEndpoints() ([]StorageEndpoint, error) {
	return nil, nil
}

func (s *fakeConfigStorage) SaveEndpoint(*StorageEndpoint) error {
	return nil
}

func (s *fakeConfigStorage) UpdateEndpoint(*StorageEndpoint) error {
	return nil
}

func (s *fakeConfigStorage) DeleteEndpoint(string) error {
	return nil
}

func (s *fakeConfigStorage) GetConfig(key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", fmt.Errorf("missing config")
	}
	return value, nil
}

func (s *fakeConfigStorage) SetConfig(key, value string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	return nil
}

func TestNormalizeProxyURLDefaultsScheme(t *testing.T) {
	got, err := NormalizeProxyURL("localhost:10808")
	if err != nil {
		t.Fatalf("NormalizeProxyURL returned error: %v", err)
	}
	if want := "http://localhost:10808"; got != want {
		t.Fatalf("NormalizeProxyURL = %q, want %q", got, want)
	}
}

func TestNormalizeProxyURLRejectsUnsupportedScheme(t *testing.T) {
	if _, err := NormalizeProxyURL("ftp://localhost:10808"); err == nil {
		t.Fatalf("expected unsupported scheme error")
	}
}

func TestLoadFromStorageNormalizesCodexProxyURL(t *testing.T) {
	cfg, err := LoadFromStorage(&fakeConfigStorage{values: map[string]string{
		"codex_proxy_url": "localhost:10808",
	}})
	if err != nil {
		t.Fatalf("LoadFromStorage returned error: %v", err)
	}
	proxy := cfg.GetCodexProxy()
	if proxy == nil {
		t.Fatalf("expected codex proxy config")
	}
	if got, want := proxy.URL, "http://localhost:10808"; got != want {
		t.Fatalf("codex proxy URL = %q, want %q", got, want)
	}
}
