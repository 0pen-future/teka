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

	// minZaloCredKeyLen is the minimum decoded key length, in bytes, Zalo
	// session credentials are encrypted at rest with. 32 bytes (256 bits)
	// matches the AES-256-GCM envelope in internal/shared/secrets.
	minZaloCredKeyLen = 32
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
	// PaceMinSeconds and PaceMaxSeconds bound the random gap between two
	// consecutive zalo_personal sends in one run. The pacing exists to keep a
	// teacher's personal account from looking like a spam bot to Zalo; 3–8s is
	// a deliberate guess (Zalo publishes no limits), which is why it is
	// configurable at all.
	PaceMinSeconds int `env:"NOTIFICATIONS_PACE_MIN_SECONDS" envDefault:"3"`
	PaceMaxSeconds int `env:"NOTIFICATIONS_PACE_MAX_SECONDS" envDefault:"8"`
	// MaxRunSize caps how many zalo_personal messages one bulk send may queue
	// for automatic delivery — the other half of the same anti-ban guardrail.
	// A larger period simply has to be sent in batches.
	MaxRunSize int `env:"NOTIFICATIONS_MAX_RUN_SIZE" envDefault:"50"`
}

// ZaloConfig configures the encryption of linked Zalo session credentials.
// Those credentials are full account-takeover material, so they are only ever
// stored sealed under this key.
type ZaloConfig struct {
	// CredKeyRaw is the configured secret exactly as read from the
	// environment — hex or base64, either is accepted (see decodeTokenKey).
	// Never logged; use CredKey for the decoded bytes.
	CredKeyRaw string `env:"ZALO_CRED_KEY"`

	// CredKey is the decoded secret, resolved by validateZalo. Not an
	// environment field itself (env:"-"): production requires CredKeyRaw to
	// decode to at least minZaloCredKeyLen bytes, because a changed key makes
	// every already-linked account undecryptable. Every other environment
	// falls back to a random per-process key when it does not.
	CredKey []byte `env:"-"`
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
	Zalo          ZaloConfig
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
	if err := c.validateNotifications(); err != nil {
		return err
	}
	if err := c.validateStatements(); err != nil {
		return err
	}
	return c.validateZalo()
}

// validateNotifications guards the zalo_personal pacing guardrail: a zero or
// negative gap, an inverted range, or a boundless run would defeat the whole
// point of pacing, so a config that asks for one is a startup error rather
// than a silently "fast" run.
func (c *Config) validateNotifications() error {
	n := c.Notifications
	if n.PaceMinSeconds < 1 {
		return fmt.Errorf("API_NOTIFICATIONS_PACE_MIN_SECONDS must be at least 1, got %d", n.PaceMinSeconds)
	}
	if n.PaceMaxSeconds < n.PaceMinSeconds {
		return fmt.Errorf("API_NOTIFICATIONS_PACE_MAX_SECONDS must be >= the minimum pace (%d), got %d", n.PaceMinSeconds, n.PaceMaxSeconds)
	}
	if n.MaxRunSize < 1 {
		return fmt.Errorf("API_NOTIFICATIONS_MAX_RUN_SIZE must be at least 1, got %d", n.MaxRunSize)
	}
	return nil
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

// validateZalo resolves Zalo.CredKey from CredKeyRaw. In production a missing
// or short key is fatal: it is the only key that can decrypt already-stored
// session credentials, so starting under a substitute would silently orphan
// every teacher's linked account and force them all to re-scan a QR code.
// Outside production, a missing or short key falls back to a random 32-byte
// key generated once for this process — links made in a previous run stop
// working, which is the accepted development trade-off. Only a fingerprint
// (never the key) is logged.
func (c *Config) validateZalo() error {
	key := decodeTokenKey(c.Zalo.CredKeyRaw)
	if len(key) >= minZaloCredKeyLen {
		c.Zalo.CredKey = key
		return nil
	}
	if c.IsProduction() {
		return fmt.Errorf("API_ZALO_CRED_KEY must be at least %d bytes", minZaloCredKeyLen)
	}

	fallback := make([]byte, minZaloCredKeyLen)
	if _, err := rand.Read(fallback); err != nil {
		return fmt.Errorf("generate development zalo credential key: %w", err)
	}
	c.Zalo.CredKey = fallback
	fingerprint := sha256.Sum256(fallback)
	slog.Warn("insecure development zalo credential key generated; previously linked accounts will not decrypt",
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
