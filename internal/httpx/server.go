package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"streamclone/internal/log"
	"streamclone/internal/metrics"
)

type ReadyFunc func(context.Context) error

type Server struct {
	Router  *chi.Mux
	log     *slog.Logger
	service string
	addr    string
	ready   []ReadyFunc
}

func New(service, addr string, logger *slog.Logger, mws ...func(http.Handler) http.Handler) *Server {
	r := chi.NewRouter()
	s := &Server{Router: r, log: logger, service: service, addr: addr}
	r.Use(log.Middleware)
	for _, m := range mws {
		r.Use(m)
	}
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", s.readyHandler)
	r.Handle("/metrics", promhttp.Handler())
	return s
}

func (s *Server) AddReady(f ReadyFunc) { s.ready = append(s.ready, f) }

func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	for _, f := range s.ready {
		if err := f(ctx); err != nil {
			metrics.ReadinessFailures.WithLabelValues(s.service).Inc()
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) Run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{
		Addr:              s.addr,
		Handler:           s.Router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		s.log.Info("listening", "addr", s.addr)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		sh, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(sh)
	}
}
