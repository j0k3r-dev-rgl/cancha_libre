package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"canchalibre/core/internal/platform/config"
	"canchalibre/core/internal/platform/postgres"
)

// newLogger builds the process logger: JSON to the given writer, at the configured
// level. The level is the operator's knob, so it is passed in rather than assumed,
// and the writer is a parameter so a test can observe what each level emits.
func newLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}

func main() {
	// Configuration is read before the logger exists, so a configuration failure is
	// reported by the standard logger and a valid LOG_LEVEL is what the process
	// actually runs with.
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	logger := newLogger(os.Stdout, cfg.LogLevel)
	slog.SetDefault(logger)

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 30*time.Second)
	pool, err := postgres.ResetAndConnect(startupCtx, postgres.ResetOptions{
		DatabaseURL:      cfg.DatabaseURL,
		SeedTopologyPath: cfg.SeedTopologyPath,
		AccountsFilePath: cfg.AccountsFilePath,
	})
	cancelStartup()
	if err != nil {
		slog.Error("failed to reset and seed database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	router, _ := newRouter(pool, cfg, logger)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	serverCtx, stopServer := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		slog.Info("shutdown signal received, initiating graceful teardown")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("error during server shutdown", "error", err)
		}

		stopServer()
	}()

	slog.Info("starting CanchaLibre core API", "port", cfg.Port, "env", cfg.Env, "log_level", cfg.LogLevel.String(), "metrics_enabled", cfg.MetricsToken != "")

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server stopped unexpectedly", "error", err)
		os.Exit(1)
	}

	<-serverCtx.Done()
	slog.Info("server gracefully stopped")
}
