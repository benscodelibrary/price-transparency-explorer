// Command explorer ingests the Transparency in Coverage files referenced by
// the configured index and serves the search API.
//
// The HTTP server starts immediately; ingest runs in the background and the
// store is published atomically when it is ready. Until then /api/v1/rates
// answers 503 and /api/v1/status reports progress.
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

	"price-explorer/internal/api"
	"price-explorer/internal/config"
	"price-explorer/internal/fetch"
	"price-explorer/internal/ingest"
	"price-explorer/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return err
	}
	log := newLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	holder := &store.Holder{}
	tracker := ingest.NewTracker()
	runner := &ingest.Runner{
		Cfg:     ingest.Config{IndexURL: cfg.IndexURL, IndexFile: cfg.IndexFile, Sources: cfg.Sources, Parallel: cfg.Parallel},
		Fetcher: fetch.New(cfg.CacheDir, log),
		Tracker: tracker,
		Log:     log,
	}
	go func() {
		start := time.Now()
		s, err := runner.Run(ctx)
		if err != nil {
			log.Error("ingest failed; API will answer 503", "err", err)
			return
		}
		holder.Store(s)
		log.Info("ingest complete, serving searches", "state", tracker.Snapshot().State, "took", time.Since(start).Round(time.Millisecond))
	}()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           (&api.Server{Holder: holder, Tracker: tracker, Log: log, WebDir: cfg.WebDir, CORSOrigins: cfg.CORSOrigins}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr, "sources", cfg.Sources)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}
