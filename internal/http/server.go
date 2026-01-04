package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/config"
	"github.com/rasorp/smuggle/internal/log"
)

// Server represents an HTTP server for the smuggle agent
type Server struct {
	cfg    *config.HTTPConfig
	logger *log.Logger
	server *http.Server
}

// New creates a new HTTP server
func New(cfg *config.HTTPConfig, logger *zap.Logger) *Server {

	s := &Server{
		cfg:    cfg,
		logger: logger.Named(log.ComponentNameHTTP),
	}

	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Address, cfg.Port),
		Handler:      s.setupRouter(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

func (s *Server) setupRouter() *chi.Mux {

	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(loggerMiddleware(s.logger, s.cfg.AccessLogLevel))

	r.Route("/v1", func(r chi.Router) {

		s.logger.Debug("setting up system endpoint routes")
		healthEndpoint := &endpointSystem{logger: s.logger}
		healthEndpoint.registerSystemRoutes(r)

		// If the operator has enabled debug endpoints, set up the Chi
		// middleware that handles this.
		if s.cfg.IsDebugEnabled() {
			s.logger.Debug("setting up debug endpoint routes")
			r.Mount("/debug", middleware.Profiler())
		}
	})

	return r
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.logger.Info("starting HTTP server", zap.String("address", s.server.Addr))

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	return nil
}

// Shutdown gracefully shuts down the HTTP server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down HTTP server")
	return s.server.Shutdown(ctx)
}
