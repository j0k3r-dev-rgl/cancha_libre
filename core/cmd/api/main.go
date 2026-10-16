package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"canchalibre/core/internal/platform/config"
	platformhttp "canchalibre/core/internal/platform/http"
	"canchalibre/core/internal/platform/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

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

	router, _ := platformhttp.NewRouter()

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

	slog.Info("starting CanchaLibre core API", "port", cfg.Port, "env", cfg.Env)

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server stopped unexpectedly", "error", err)
		os.Exit(1)
	}

	<-serverCtx.Done()
	slog.Info("server gracefully stopped")
}
