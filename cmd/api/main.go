package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alex/easy-chess-results-api/internal/api"
	"github.com/alex/easy-chess-results-api/internal/config"
	"github.com/alex/easy-chess-results-api/internal/service"
	"github.com/alex/easy-chess-results-api/internal/store"
	"github.com/alex/easy-chess-results-api/internal/upstream"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fail("invalid configuration", err)
	}
	level := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		level = slog.LevelDebug
	} else if cfg.LogLevel == "warn" {
		level = slog.LevelWarn
	} else if cfg.LogLevel == "error" {
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if cfg.LogFormat == "text" {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	log := slog.New(h)
	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		fail("open database", err)
	}
	defer st.Close()
	up, err := upstream.New(cfg.UpstreamBaseURL, cfg.UpstreamLanguage, cfg.UpstreamTimeout, cfg.UpstreamMaxConcurrency, cfg.UpstreamMinInterval, cfg.UpstreamMaxBodyBytes)
	if err != nil {
		fail("configure upstream", err)
	}
	svc := service.New(cfg, st, up)
	server := &http.Server{Addr: cfg.ListenAddr, Handler: api.New(cfg, svc, st, log), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	go func() {
		log.Info("server starting", "address", cfg.ListenAddr)
		if e := server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			fail("serve", e)
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err = server.Shutdown(shutdown); err != nil {
		log.Error("graceful shutdown failed", "error", err)
	}
	log.Info("server stopped")
}
func fail(message string, err error) { slog.Error(message, "error", err); os.Exit(1) }
