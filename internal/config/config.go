package config

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
)

// Config holds all configuration for the stashkit service.
type Config struct {
	Addr     string
	DataDir  string
	Secret   string
	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string
}

// Default returns a config with sensible defaults.
func Default() *Config {
	return &Config{
		Addr:    ":7788",
		DataDir: "./data",
		Secret:  "",
	}
}

// Load parses flags and environment variables, layering them over defaults.
// Order: defaults < env vars < flags
func Load() *Config {
	c := Default()

	// Environment variables
	if v := os.Getenv("STASHKIT_ADDR"); v != "" {
		c.Addr = v
	}
	if v := os.Getenv("STASHKIT_DATA"); v != "" {
		c.DataDir = v
	}
	if v := os.Getenv("STASHKIT_SECRET"); v != "" {
		c.Secret = v
	}
	if v := os.Getenv("STASHKIT_SMTP_HOST"); v != "" {
		c.SMTPHost = v
	}
	if v := os.Getenv("STASHKIT_SMTP_PORT"); v != "" {
		c.SMTPPort = v
	}
	if v := os.Getenv("STASHKIT_SMTP_USER"); v != "" {
		c.SMTPUser = v
	}
	if v := os.Getenv("STASHKIT_SMTP_PASS"); v != "" {
		c.SMTPPass = v
	}
	if v := os.Getenv("STASHKIT_SMTP_FROM"); v != "" {
		c.SMTPFrom = v
	}

	// Flags (override env)
	flag.StringVar(&c.Addr, "addr", c.Addr, "listen address")
	flag.StringVar(&c.DataDir, "data", c.DataDir, "data directory for JSON storage")
	flag.StringVar(&c.Secret, "secret", c.Secret, "token signing secret (auto-generated if empty)")
	flag.StringVar(&c.SMTPHost, "smtp-host", c.SMTPHost, "SMTP server host")
	flag.StringVar(&c.SMTPPort, "smtp-port", c.SMTPPort, "SMTP server port")
	flag.StringVar(&c.SMTPUser, "smtp-user", c.SMTPUser, "SMTP username")
	flag.StringVar(&c.SMTPPass, "smtp-pass", c.SMTPPass, "SMTP password")
	flag.StringVar(&c.SMTPFrom, "smtp-from", c.SMTPFrom, "SMTP from address")
	flag.Parse()

	// Auto-generate secret if not provided
	if c.Secret == "" {
		c.Secret = generateSecret()
	}

	return c
}

// generateSecret creates a random 32-byte hex secret.
func generateSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// String returns a human-readable config summary.
func (c *Config) String() string {
	return fmt.Sprintf("addr=%s data=%s smtp=%s", c.Addr, c.DataDir, c.SMTPHost)
}
