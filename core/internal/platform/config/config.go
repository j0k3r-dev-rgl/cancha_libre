package config

import (
	"errors"
	"fmt"
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

	return Config{
		Port:              port,
		Env:               env,
		DatabaseURL:       dbURL,
		JWTSecret:         jwtSecret,
		InvitationURLBase: invBase,
		SeedTopologyPath:  seedPath,
		AccountsFilePath:  accountsPath,
	}, nil
}
