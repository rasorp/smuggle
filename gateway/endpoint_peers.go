package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type endpointPeers struct {
	gateway *Gateway
	logger  *zap.Logger
}

func (e *endpointPeers) registerRoutes(r chi.Router) {
	r.Post("/gateway/peers", e.createPeer)
	r.Delete("/gateway/peers/{public_key}", e.deletePeer)
}

// ── POST /v1/gateway/peers ────────────────────────────────────────────────────

type createPeerRequest struct {
	PublicKey string `json:"public_key"`
}

type createPeerResponse struct {
	GatewayPublicKey    string `json:"gateway_public_key"`
	GatewayEndpoint     string `json:"gateway_endpoint"`
	CustomerTunnelIP    string `json:"customer_tunnel_ip"`
	OverlayCIDR         string `json:"overlay_cidr"`
	PersistentKeepalive int    `json:"persistent_keepalive"`
}

func (e *endpointPeers) createPeer(w http.ResponseWriter, r *http.Request) {
	var req createPeerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.PublicKey == "" {
		writeError(w, http.StatusBadRequest, "public_key is required")
		return
	}
	if _, err := wgtypes.ParseKey(req.PublicKey); err != nil {
		writeError(w, http.StatusBadRequest, "invalid WireGuard public key")
		return
	}

	if _, exists := e.gateway.registry.Get(req.PublicKey); exists {
		writeError(w, http.StatusConflict, "peer already registered")
		return
	}

	tunnelIP, err := e.gateway.registry.Allocate(req.PublicKey)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "tunnel IP pool exhausted")
		return
	}

	if err := e.gateway.addPeer(req.PublicKey, tunnelIP); err != nil {
		// Roll back the allocation so the IP is not stranded.
		if _, releaseErr := e.gateway.registry.Release(req.PublicKey); releaseErr != nil {
			e.logger.Error("failed to release IP after peer add failure",
				zap.String("public_key", req.PublicKey),
				zap.Error(releaseErr),
			)
		}
		e.logger.Error("failed to configure peer", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to configure peer")
		return
	}

	e.logger.Info("registered customer peer",
		zap.String("public_key", req.PublicKey),
		zap.String("tunnel_ip", tunnelIP.String()),
	)

	writeJSON(w, http.StatusCreated, createPeerResponse{
		GatewayPublicKey:    e.gateway.publicKey,
		GatewayEndpoint:     fmt.Sprintf("%s:%d", e.gateway.cfg.EndpointIP, e.gateway.cfg.WGPort),
		CustomerTunnelIP:    tunnelIP.String(),
		OverlayCIDR:         e.gateway.cfg.OverlayCIDR,
		PersistentKeepalive: 25,
	})
}

// ── DELETE /v1/gateway/peers/{public_key} ─────────────────────────────────────

func (e *endpointPeers) deletePeer(w http.ResponseWriter, r *http.Request) {
	// The public key is base64; the + character is often URL-encoded as %2B.
	rawKey := chi.URLParam(r, "public_key")
	publicKey, err := url.QueryUnescape(rawKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid public key encoding")
		return
	}

	if _, err := wgtypes.ParseKey(publicKey); err != nil {
		writeError(w, http.StatusBadRequest, "invalid WireGuard public key")
		return
	}

	tunnelIP, err := e.gateway.registry.Release(publicKey)
	if err != nil {
		writeError(w, http.StatusNotFound, "peer not found")
		return
	}

	if err := e.gateway.removePeer(publicKey, tunnelIP); err != nil {
		e.logger.Error("failed to remove peer", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to remove peer")
		return
	}

	e.logger.Info("removed customer peer",
		zap.String("public_key", publicKey),
		zap.String("tunnel_ip", tunnelIP.String()),
	)

	w.WriteHeader(http.StatusNoContent)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
