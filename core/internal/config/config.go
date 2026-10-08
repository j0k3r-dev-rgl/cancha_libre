package config

import (
	"os"
)

// Config holds runtime configuration loaded from environment variables.
type Config struct {
	Port        string
	Env         string
	DatabaseURL string
	JWTSecret   string
}

// Load reads configuration from the environment, applying sensible development defaults.
func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	env := os.Getenv("ENV")
	if env == "" {
		env = "development"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/canchalibre_dev?sslmode=disable"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "super-secret-development-key-change-in-production"
	}

	return Config{
		Port:        port,
		Env:         env,
		DatabaseURL: dbURL,
		JWTSecret:   jwtSecret,
	}
}
