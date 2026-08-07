package config

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
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
	if cfg.Notifications.DefaultChannel != "zalo_manual" {
		t.Errorf("Notifications.DefaultChannel = %q, want zalo_manual", cfg.Notifications.DefaultChannel)
	}
	if cfg.Notifications.MaxMessageLen != 1000 {
		t.Errorf("Notifications.MaxMessageLen = %d, want 1000", cfg.Notifications.MaxMessageLen)
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
		{
			name: "missing statements token key in production",
			mutate: func(t *testing.T) {
				t.Setenv("API_ENV", EnvProduction)
			},
			wantSub: "API_STATEMENTS_TOKEN_KEY",
		},
		{
			name: "short statements token key in production",
			mutate: func(t *testing.T) {
				t.Setenv("API_ENV", EnvProduction)
				t.Setenv("API_STATEMENTS_TOKEN_KEY", "too-short")
			},
			wantSub: "API_STATEMENTS_TOKEN_KEY",
		},
		{
			name: "missing zalo credential key in production",
			mutate: func(t *testing.T) {
				t.Setenv("API_ENV", EnvProduction)
				t.Setenv("API_STATEMENTS_TOKEN_KEY", strings.Repeat("ab", minStatementTokenKeyLen))
			},
			wantSub: "API_ZALO_CRED_KEY",
		},
		{
			name: "short zalo credential key in production",
			mutate: func(t *testing.T) {
				t.Setenv("API_ENV", EnvProduction)
				t.Setenv("API_STATEMENTS_TOKEN_KEY", strings.Repeat("ab", minStatementTokenKeyLen))
				t.Setenv("API_ZALO_CRED_KEY", "too-short")
			},
			wantSub: "API_ZALO_CRED_KEY",
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

// TestStatementsTokenKeyDevFallback proves a development/test process never
// fails to start over a missing statement token key — it gets a working
// random key instead, long enough for deriveToken/hashToken.
func TestStatementsTokenKeyDevFallback(t *testing.T) {
	setRequired(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Statements.TokenKey) < minStatementTokenKeyLen {
		t.Errorf("Statements.TokenKey length = %d, want >= %d", len(cfg.Statements.TokenKey), minStatementTokenKeyLen)
	}
}

// TestStatementsTokenKeyDecodesConfiguredValue proves both documented
// encodings (hex and base64) decode to a usable key rather than falling back.
func TestStatementsTokenKeyDecodesConfiguredValue(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"hex", strings.Repeat("ab", minStatementTokenKeyLen)},
		{"base64", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", minStatementTokenKeyLen)))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setRequired(t)
			t.Setenv("API_STATEMENTS_TOKEN_KEY", tc.raw)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if len(cfg.Statements.TokenKey) < minStatementTokenKeyLen {
				t.Errorf("Statements.TokenKey length = %d, want >= %d", len(cfg.Statements.TokenKey), minStatementTokenKeyLen)
			}
		})
	}
}

// TestZaloCredKeyDevFallback proves a development/test process still starts
// without a configured Zalo credential key — it gets a random per-process one.
// Every account linked under that key becomes unreadable on the next start,
// which is the intended development trade-off.
func TestZaloCredKeyDevFallback(t *testing.T) {
	setRequired(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Zalo.CredKey) < minZaloCredKeyLen {
		t.Errorf("Zalo.CredKey length = %d, want >= %d", len(cfg.Zalo.CredKey), minZaloCredKeyLen)
	}
}

// TestZaloCredKeyDecodesConfiguredValue proves both documented encodings
// decode to a usable key rather than silently falling back to a random one —
// a fallback in production would orphan every linked account.
func TestZaloCredKeyDecodesConfiguredValue(t *testing.T) {
	want := strings.Repeat("k", minZaloCredKeyLen)
	cases := []struct {
		name string
		raw  string
	}{
		{"hex", hex.EncodeToString([]byte(want))},
		{"base64", base64.StdEncoding.EncodeToString([]byte(want))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setRequired(t)
			t.Setenv("API_ZALO_CRED_KEY", tc.raw)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if string(cfg.Zalo.CredKey) != want {
				t.Errorf("Zalo.CredKey = %q, want the decoded configured key", cfg.Zalo.CredKey)
			}
		})
	}
}

// TestZaloCredKeyIsStableAcrossLoads proves a configured key decodes to the
// same bytes every start: rotating it silently would make every stored
// credential undecryptable, so the value must never drift.
func TestZaloCredKeyIsStableAcrossLoads(t *testing.T) {
	setRequired(t)
	t.Setenv("API_ZALO_CRED_KEY", hex.EncodeToString([]byte(strings.Repeat("z", minZaloCredKeyLen))))

	first, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	second, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !bytes.Equal(first.Zalo.CredKey, second.Zalo.CredKey) {
		t.Error("Zalo.CredKey differs between loads of the same configuration")
	}
}
