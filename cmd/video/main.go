package main

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"streamclone/internal/config"
	"streamclone/internal/httpx"
	"streamclone/internal/log"
	"streamclone/internal/metrics"
	"streamclone/internal/video/orchestrator"
	"streamclone/internal/video/registry"
	"streamclone/internal/video/token"
	"streamclone/internal/video/usher"
	"streamclone/internal/video/worker"
)

func main() {
	cfg, err := config.Load()
	logger := log.New("video", cfg.LogLevel)
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}

	if n := worker.Reconcile("rtmp://" + cfg.MediaMTXRTMP); n > 0 {
		logger.Warn("reconciled orphan stream processes", "killed", n)
	}

	orch := orchestrator.New(orchestrator.Options{
		Token:          token.New(cfg.Upstream),
		Usher:          usher.New(cfg.Upstream),
		Registry:       registry.New(),
		Log:            logger,
		RTMPBase:       cfg.MediaMTXRTMP,
		HLSBase:        cfg.HLSPublicBase,
		HLSProbeBase:   cfg.HLSInternalBase,
		MaxStreams:     cfg.MaxConcurrentStreams,
		MaxRelays:      cfg.MaxConcurrentRelays,
		IdleTimeout:    cfg.StreamIdleTimeout,
		BackendVersion: cfg.BackendVersion,
		WorkerBackends: strings.Split(cfg.StreamWorkerBackends, ","),
		DefaultQuality: cfg.DefaultStreamQuality,
	})

	srv := httpx.New("video", cfg.HTTPAddr, logger, metrics.HTTPMiddleware("video"), httpx.CORS, httpx.NewRateLimiter(20, 40).Middleware)
	srv.AddReady(func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(cfg.HLSInternalBase, "/"), nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		return resp.Body.Close()
	})
	orch.Routes(srv.Router)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go orch.RunReaper(ctx, 10*time.Second)

	if err := srv.Run(ctx); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
