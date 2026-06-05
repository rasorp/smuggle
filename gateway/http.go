package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// HTTPServer wraps a net/http.Server with chi routing and zap access logging.
type HTTPServer struct {
	server  *http.Server
	logger  *zap.Logger
	gateway *Gateway
}

// NewHTTPServer creates a configured HTTPServer. Call Start to begin accepting
// connections.
func NewHTTPServer(addr string, gateway *Gateway, logger *zap.Logger) *HTTPServer {
	s := &HTTPServer{
		logger:  logger.Named("http"),
		gateway: gateway,
	}

	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.setupRouter(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

func (s *HTTPServer) setupRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(s.accessLogger())

	r.Route("/v1", func(r chi.Router) {
		r.Get("/system/health", s.getHealth)

		ep := &endpointPeers{gateway: s.gateway, logger: s.logger}
		ep.registerRoutes(r)
	})

	return r
}

// accessLogger returns a chi middleware that logs each request at Debug level.
func (s *HTTPServer) accessLogger() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			s.logger.Log(zapcore.DebugLevel, "request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", ww.Status()),
				zap.Duration("duration", time.Since(start)),
			)
		})
	}
}

func (s *HTTPServer) getHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, `{"status":"ok"}`)
}

// Start begins serving in a background goroutine.
func (s *HTTPServer) Start() error {
	s.logger.Info("starting HTTP server", zap.String("addr", s.server.Addr))
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server error", zap.Error(err))
		}
	}()
	return nil
}

// Shutdown gracefully drains in-flight requests.
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down HTTP server")
	return s.server.Shutdown(ctx)
}
