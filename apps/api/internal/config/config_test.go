package config

import (
	"strings"
	"testing"
	"time"
)

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("API_ENV", EnvTest)
	t.Setenv("API_DATABASE_URL", "postgres://u:p@localhost:5432/db?sslmode=disable")
	t.Setenv("API_JWT_SECRET", strings.Repeat("s", minJWTSecretLen))
}

func TestLoadDefaults(t *testing.T) {
	setRequired(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTP.Port != 8080 {
		t.Errorf("HTTP.Port = %d, want 8080", cfg.HTTP.Port)
	}
	if cfg.JWT.AccessTTL != 15*time.Minute {
		t.Errorf("JWT.AccessTTL = %v, want 15m", cfg.JWT.AccessTTL)
	}
	if cfg.JWT.RefreshTTL != 720*time.Hour {
		t.Errorf("JWT.RefreshTTL = %v, want 720h", cfg.JWT.RefreshTTL)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if len(cfg.CORSOrigins) != 1 || cfg.CORSOrigins[0] != "http://localhost:5173" {
		t.Errorf("CORSOrigins = %v, want default localhost:5173", cfg.CORSOrigins)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(t *testing.T)
		wantSub string
	}{
		{
			name:    "missing database url",
			mutate:  func(t *testing.T) { t.Setenv("API_DATABASE_URL", "") },
			wantSub: "API_DATABASE_URL",
		},
		{
			name:    "short jwt secret",
			mutate:  func(t *testing.T) { t.Setenv("API_JWT_SECRET", "short") },
			wantSub: "API_JWT_SECRET",
		},
		{
			name:    "invalid env",
			mutate:  func(t *testing.T) { t.Setenv("API_ENV", "staging") },
			wantSub: "API_ENV",
		},
		{
			name:    "invalid log level",
			mutate:  func(t *testing.T) { t.Setenv("API_LOG_LEVEL", "verbose") },
			wantSub: "API_LOG_LEVEL",
		},
		{
			name:    "wildcard cors origin",
			mutate:  func(t *testing.T) { t.Setenv("API_CORS_ORIGINS", "*") },
			wantSub: "API_CORS_ORIGINS",
		},
		{
			name:    "malformed cors origin",
			mutate:  func(t *testing.T) { t.Setenv("API_CORS_ORIGINS", "http://ok.example,bad.example") },
			wantSub: "API_CORS_ORIGINS",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setRequired(t)
			tc.mutate(t)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not mention %q", err, tc.wantSub)
			}
		})
	}
}

func TestParsesOverrides(t *testing.T) {
	setRequired(t)
	t.Setenv("API_HTTP_PORT", "9999")
	t.Setenv("API_CORS_ORIGINS", "https://a.example,https://b.example")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTP.Port != 9999 {
		t.Errorf("HTTP.Port = %d, want 9999", cfg.HTTP.Port)
	}
	if len(cfg.CORSOrigins) != 2 {
		t.Errorf("CORSOrigins = %v, want 2 entries", cfg.CORSOrigins)
	}
}
