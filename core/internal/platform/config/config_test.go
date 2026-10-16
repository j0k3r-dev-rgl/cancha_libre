package config_test

import (
	"log/slog"
	"strings"
	"testing"

	"canchalibre/core/internal/platform/config"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing, got nil")
	}
}

func TestLoadRequiresJWTSecretOfAtLeast32Bytes(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5435/canchalibre_dev?sslmode=disable")
	t.Setenv("JWT_SECRET", "too-short")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when JWT_SECRET is shorter than 32 bytes, got nil")
	}
}

func TestLoadSucceedsWithValidEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5435/canchalibre_dev?sslmode=disable")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("PORT", "")
	t.Setenv("ENV", "")
	t.Setenv("LOG_LEVEL", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Fatalf("expected default port 8080, got %q", cfg.Port)
	}
	if cfg.Env != "development" {
		t.Fatalf("expected default env development, got %q", cfg.Env)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("expected default log level info, got %v", cfg.LogLevel)
	}
}

// TestParseLogLevelNamesTheAcceptedValues pins the operator-facing contract of
// LOG_LEVEL: four names, case-insensitive, an empty value is the info default, and
// anything else is a startup failure that names what is accepted rather than a
// silent fallback that would hide the logs the operator asked for.
func TestParseLogLevelNamesTheAcceptedValues(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		raw     string
		want    slog.Level
		wantErr bool
	}{
		{raw: "", want: slog.LevelInfo},
		{raw: "  ", want: slog.LevelInfo},
		{raw: "debug", want: slog.LevelDebug},
		{raw: "DEBUG", want: slog.LevelDebug},
		{raw: "info", want: slog.LevelInfo},
		{raw: "warn", want: slog.LevelWarn},
		{raw: ".WARN", wantErr: true},
		{raw: "error", want: slog.LevelError},
		{raw: "loud", wantErr: true},
	} {
		got, err := config.ParseLogLevel(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("ParseLogLevel(%q) = %v, want an error", tc.raw, got)
			}
			if !strings.Contains(err.Error(), "LOG_LEVEL") {
				t.Fatalf("ParseLogLevel(%q) error %q does not name LOG_LEVEL", tc.raw, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseLogLevel(%q) unexpected error: %v", tc.raw, err)
		}
		if got != tc.want {
			t.Fatalf("ParseLogLevel(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

// TestLoadLeavesMetricsDisabledWithoutMetricsToken pins the switch the endpoint
// hangs from: an unset (or empty) METRICS_TOKEN is a valid configuration that
// disables metrics, not a startup failure.
func TestLoadLeavesMetricsDisabledWithoutMetricsToken(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5435/canchalibre_dev?sslmode=disable")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("METRICS_TOKEN", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MetricsToken != "" {
		t.Fatalf("expected metrics to be disabled, got token %q", cfg.MetricsToken)
	}
}

// TestLoadRejectsMetricsTokenShorterThan32Bytes proves a weak scrape credential
// stops the process instead of silently serving the metrics surface behind a
// token an attacker can guess.
func TestLoadRejectsMetricsTokenShorterThan32Bytes(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5435/canchalibre_dev?sslmode=disable")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("METRICS_TOKEN", "short-token")

	if _, err := config.Load(); err == nil {
		t.Fatal("expected an error for a METRICS_TOKEN shorter than 32 bytes, got nil")
	}
}

// TestLoadReadsMetricsToken keeps the operator's value exactly, so the handler
// compares against what was configured and not a normalized variant.
func TestLoadReadsMetricsToken(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef"

	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5435/canchalibre_dev?sslmode=disable")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("METRICS_TOKEN", token)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MetricsToken != token {
		t.Fatalf("expected token %q, got %q", token, cfg.MetricsToken)
	}
}

// TestLoadRejectsAnUnknownLogLevel proves the rejection reaches Load itself, so a
// typo in the environment stops the process instead of silently logging at info.
func TestLoadRejectsAnUnknownLogLevel(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5435/canchalibre_dev?sslmode=disable")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("LOG_LEVEL", "verbose")

	if _, err := config.Load(); err == nil {
		t.Fatal("expected an error for an unknown LOG_LEVEL, got nil")
	}
}
