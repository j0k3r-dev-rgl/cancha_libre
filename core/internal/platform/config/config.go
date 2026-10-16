package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// MinJWTSecretBytes is the minimum length required for the HS256 signing key.
const MinJWTSecretBytes = 32

// Config holds runtime configuration loaded from environment variables.
type Config struct {
	Port              string
	Env               string
	DatabaseURL       string
	JWTSecret         string
	InvitationURLBase string
	SeedTopologyPath  string
	AccountsFilePath  string
	LogLevel          slog.Level
}

// ParseLogLevel maps LOG_LEVEL to a structured-log level. An empty value is the
// `info` default; anything else must name one of the four levels, because a typo
// must stop the process rather than silently hide the logs the operator asked for.
func ParseLogLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("LOG_LEVEL %q is not one of debug, info, warn, error", raw)
	}
}

// Load reads configuration from the environment. DATABASE_URL and JWT_SECRET are
// mandatory and never fall back to a silent default.
func Load() (Config, error) {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}

	env := strings.TrimSpace(os.Getenv("ENV"))
	if env == "" {
		env = "development"
	}

	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		return Config{}, errors.New("DATABASE_URL is required and must not be empty")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if len(jwtSecret) < MinJWTSecretBytes {
		return Config{}, fmt.Errorf("JWT_SECRET is required and must be at least %d bytes", MinJWTSecretBytes)
	}

	invBase := strings.TrimSpace(os.Getenv("INVITATION_URL_BASE"))
	if invBase == "" {
		invBase = "http://localhost:3000/auth/activate"
	}

	seedPath := strings.TrimSpace(os.Getenv("SEED_TOPOLOGY_PATH"))
	if seedPath == "" {
		seedPath = "dev/seed.topology.json"
	}

	accountsPath := strings.TrimSpace(os.Getenv("ACCOUNTS_FILE_PATH"))
	if accountsPath == "" {
		accountsPath = "../.pi/api.json.local"
	}

	logLevel, err := ParseLogLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		return Config{}, err
	}

	return Config{
		Port:              port,
		Env:               env,
		DatabaseURL:       dbURL,
		JWTSecret:         jwtSecret,
		InvitationURLBase: invBase,
		SeedTopologyPath:  seedPath,
		AccountsFilePath:  accountsPath,
		LogLevel:          logLevel,
	}, nil
}
