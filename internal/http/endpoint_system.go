package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/log"
)

type endpointSystem struct {
	logger *log.Logger
}

func (e *endpointSystem) registerSystemRoutes(r chi.Router) {
	r.Route("/system", func(r chi.Router) {
		r.Get("/health", e.getHealth)
	})
}

type GetSystemHealthReq struct{}

type GetSystemHealthResp struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (e *endpointSystem) getHealth(w http.ResponseWriter, _ *http.Request) {
	response := GetSystemHealthResp{
		Status:  "OK",
		Message: "Smuggle agent is healthy",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		e.logger.Error("failed to encode health response", zap.Error(err))
	}
}
