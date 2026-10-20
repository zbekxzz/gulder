// Command api — HTTP-сервер Gulder. Миграции здесь не применяются: это отдельный шаг
// `cmd/migrate` перед запуском новой версии (§18.6).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zbekxzz/gulder/backend/internal/config"
	"github.com/zbekxzz/gulder/backend/internal/httpapi"
	"github.com/zbekxzz/gulder/backend/internal/metrics"
	"github.com/zbekxzz/gulder/backend/internal/ranking"
	"github.com/zbekxzz/gulder/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(httpapi.LogHandler{Handler: slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rankCfg, err := ranking.LoadConfig(cfg.RankingConfig)
	if err != nil {
		return err
	}
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.New(cfg, st, httpapi.WithRanking(rankCfg)).Router(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second, // ассистент: LLM 20 с + повтор
		IdleTimeout:       120 * time.Second,
	}
	// /metrics — на отдельном адресе, не публикуется наружу через reverse proxy (§18.4)
	if cfg.MetricsAddr != "" {
		metricsSrv := &http.Server{Addr: cfg.MetricsAddr, Handler: metrics.Handler(), ReadHeaderTimeout: 5 * time.Second}
		go func() {
			if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("metrics server stopped", "err", err)
			}
		}()
		defer func() { _ = metricsSrv.Close() }()
	}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("api listening", "addr", cfg.HTTPAddr, "env", cfg.Env)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
