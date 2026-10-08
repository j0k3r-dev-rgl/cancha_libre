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

	adapterhttp "canchalibre/core/internal/adapter/http"
	"canchalibre/core/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()

	router := adapterhttp.NewRouter()

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Server run context with signal interception for graceful shutdown
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
