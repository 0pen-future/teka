// Package config loads and validates application configuration from
// API_-prefixed environment variables, with .env support outside production.
package config

import (
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

// Config is the full application configuration, populated from API_-prefixed
// environment variables.
type Config struct {
	Env         string   `env:"ENV" envDefault:"development"`
	LogLevel    string   `env:"LOG_LEVEL" envDefault:"info"`
	CORSOrigins []string `env:"CORS_ORIGINS" envSeparator:"," envDefault:"http://localhost:5173"`

	HTTP     HTTPConfig
	Database DatabaseConfig
	JWT      JWTConfig
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
