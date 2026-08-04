// Package config loads and validates application configuration from
// API_-prefixed environment variables, with .env support outside production.
package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Valid API_ENV values.
const (
	EnvDevelopment = "development"
	EnvTest        = "test"
	EnvProduction  = "production"

	minJWTSecretLen = 32

	// minStatementTokenKeyLen is the minimum decoded key length, in bytes,
	// statement links are signed with. 32 bytes (256 bits) matches
	// deriveToken's HMAC-SHA256.
	minStatementTokenKeyLen = 32
)

// HTTPConfig configures the HTTP listener.
type HTTPConfig struct {
	Port int `env:"HTTP_PORT" envDefault:"8080"`
}

// DatabaseConfig configures the PostgreSQL connection and pool.
type DatabaseConfig struct {
	URL             string        `env:"DATABASE_URL,required,notEmpty"`
	MaxOpenConns    int           `env:"DB_MAX_OPEN_CONNS" envDefault:"25"`
	MaxIdleConns    int           `env:"DB_MAX_IDLE_CONNS" envDefault:"5"`
	ConnMaxLifetime time.Duration `env:"DB_CONN_MAX_LIFETIME" envDefault:"30m"`
}

// JWTConfig configures token signing and lifetimes.
type JWTConfig struct {
	Secret     string        `env:"JWT_SECRET,required"`
	AccessTTL  time.Duration `env:"JWT_ACCESS_TTL" envDefault:"15m"`
	RefreshTTL time.Duration `env:"JWT_REFRESH_TTL" envDefault:"720h"`
}

// StatementsConfig configures parent statement links: the secret token
// derivation is keyed on, and the base URL those links are built against.
type StatementsConfig struct {
	// TokenKeyRaw is the configured secret exactly as read from the
	// environment — hex or base64, either is accepted (see decodeTokenKey).
	// Never logged; use TokenKey for the decoded bytes deriveToken/hashToken
	// actually sign with.
	TokenKeyRaw string `env:"STATEMENTS_TOKEN_KEY"`
	// PublicBaseURL prefixes the token path ("/s/{token}") a generated
	// statement link points at.
	PublicBaseURL string `env:"STATEMENTS_PUBLIC_BASE_URL" envDefault:"http://localhost:5173"`

	// TokenKey is the decoded secret, resolved by validateStatements. Not an
	// environment field itself (env:"-"): production requires TokenKeyRaw to
	// decode to at least minStatementTokenKeyLen bytes; every other
	// environment falls back to a random per-process key when it does not,
	// so a fresh key each run never leaks into logs or version control.
	TokenKey []byte `env:"-"`
}

// BankConfig is the teacher's transfer target used to render VietQR payment
// codes on public statements (see the statements package's QRBuilder). The
// schema has no column for one in V1 — a single teacher-wide account is
// enough for a solo-teacher tenant — so it is read from application
// configuration instead. Every field is optional and unvalidated: an
// unconfigured account is a supported state (the QR block is simply omitted
// from a statement, never faked), not a startup error.
type BankConfig struct {
	BankCode      string `env:"BANK_CODE"`
	AccountNumber string `env:"BANK_ACCOUNT_NUMBER"`
	AccountName   string `env:"BANK_ACCOUNT_NAME"`
}

// NotificationsConfig configures parent notification sends: the channel a
// bulk send uses when the request does not specify one, and the character
// ceiling a rendered message collapses to fit under (see the statements
// package's Build).
type NotificationsConfig struct {
	// DefaultChannel is the channel a bulk send uses when the request omits
	// one — zalo_manual is V1's only wired sender.
	DefaultChannel string `env:"NOTIFICATIONS_DEFAULT_CHANNEL" envDefault:"zalo_manual"`
	// MaxMessageLen is the character ceiling a rendered message must fit
	// under before it collapses its per-child detail (statements.Build's
	// maxLen).
	MaxMessageLen int `env:"NOTIFICATIONS_MAX_MESSAGE_LEN" envDefault:"1000"`
}

// Config is the full application configuration, populated from API_-prefixed
// environment variables.
type Config struct {
	Env         string   `env:"ENV" envDefault:"development"`
	LogLevel    string   `env:"LOG_LEVEL" envDefault:"info"`
	CORSOrigins []string `env:"CORS_ORIGINS" envSeparator:"," envDefault:"http://localhost:5173"`

	HTTP          HTTPConfig
	Database      DatabaseConfig
	JWT           JWTConfig
	Statements    StatementsConfig
	Bank          BankConfig
	Notifications NotificationsConfig
}

// Load reads configuration from the environment (prefix API_). In development
// (the default when API_ENV is unset) it first loads a .env file when one is
// present — either in the current directory or at the repo root when running
// from apps/api. Test and production read the process environment only, so
// tests stay hermetic.
func Load() (*Config, error) {
	if e := os.Getenv("API_ENV"); e == "" || e == EnvDevelopment {
		for _, path := range []string{".env", "../../.env"} {
			if _, err := os.Stat(path); err == nil {
				_ = godotenv.Load(path)
				break
			}
		}
	}

	cfg := &Config{}
	if err := env.ParseWithOptions(cfg, env.Options{Prefix: "API_"}); err != nil {
		return nil, fmt.Errorf("parse environment: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	switch c.Env {
	case EnvDevelopment, EnvTest, EnvProduction:
	default:
		return fmt.Errorf("API_ENV must be one of %s|%s|%s, got %q", EnvDevelopment, EnvTest, EnvProduction, c.Env)
	}
	if len(c.JWT.Secret) < minJWTSecretLen {
		return fmt.Errorf("API_JWT_SECRET must be at least %d characters", minJWTSecretLen)
	}
	if _, err := parseLogLevel(c.LogLevel); err != nil {
		return err
	}
	for _, origin := range c.CORSOrigins {
		// The API always allows credentials, which browsers reject for the
		// wildcard origin; and gin-contrib/cors panics on malformed origins
		// at router build time — catch both here for a clean startup error.
		if origin == "*" {
			return fmt.Errorf("API_CORS_ORIGINS must list explicit origins; %q is incompatible with credentialed requests", origin)
		}
		if !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
			return fmt.Errorf("API_CORS_ORIGINS entry %q must start with http:// or https://", origin)
		}
	}
	return c.validateStatements()
}

// decodeTokenKey resolves a configured secret into raw key bytes. It tries
// hex first (the shape `openssl rand -hex 32` produces), then standard and
// URL-safe base64 (the shape `openssl rand -base64 32` produces — the form
// .env.example documents), and finally falls back to the string's own bytes
// so any sufficiently long random string still works. Only length is
// enforced here; validateStatements decides whether that length is
// acceptable for the running environment.
func decodeTokenKey(raw string) []byte {
	if raw == "" {
		return nil
	}
	if b, err := hex.DecodeString(raw); err == nil {
		return b
	}
	if b, err := base64.StdEncoding.DecodeString(raw); err == nil {
		return b
	}
	if b, err := base64.RawURLEncoding.DecodeString(raw); err == nil {
		return b
	}
	return []byte(raw)
}

// validateStatements resolves Statements.TokenKey from TokenKeyRaw. In
// production a missing or short key is fatal — rotating it invalidates every
// parent statement link already sent out, so it must be deliberate and
// stable, never silently substituted. Outside production, a missing or short
// key falls back to a random 32-byte key generated once for this process;
// only a fingerprint (never the key) is logged, so a developer immediately
// knows a real key was not configured without a secret ever reaching stdout.
func (c *Config) validateStatements() error {
	key := decodeTokenKey(c.Statements.TokenKeyRaw)
	if len(key) >= minStatementTokenKeyLen {
		c.Statements.TokenKey = key
		return nil
	}
	if c.IsProduction() {
		return fmt.Errorf("API_STATEMENTS_TOKEN_KEY must be at least %d bytes", minStatementTokenKeyLen)
	}

	fallback := make([]byte, minStatementTokenKeyLen)
	if _, err := rand.Read(fallback); err != nil {
		return fmt.Errorf("generate development statement token key: %w", err)
	}
	c.Statements.TokenKey = fallback
	fingerprint := sha256.Sum256(fallback)
	slog.Warn("insecure development statement token key generated",
		"fingerprint", hex.EncodeToString(fingerprint[:])[:8])
	return nil
}

// IsProduction reports whether the app runs in production mode.
func (c *Config) IsProduction() bool { return c.Env == EnvProduction }

// IsDevelopment reports whether the app runs in development mode.
func (c *Config) IsDevelopment() bool { return c.Env == EnvDevelopment }

// SlogLevel returns the configured log level; validation guarantees it parses.
func (c *Config) SlogLevel() slog.Level {
	lvl, _ := parseLogLevel(c.LogLevel)
	return lvl
}

func parseLogLevel(s string) (slog.Level, error) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(s)); err != nil {
		return 0, fmt.Errorf("API_LOG_LEVEL must be debug|info|warn|error, got %q", s)
	}
	return lvl, nil
}
