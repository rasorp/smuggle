package http

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/shoenig/test/must"
	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/config"
	"github.com/rasorp/smuggle/internal/helper"
)

// newTestServer creates a Server bound to the given address:port, using a
// no-op zap logger so tests produce no log output.
func newTestServer(t *testing.T, addr string, port uint) *Server {
	t.Helper()

	cfg := &config.HTTPConfig{
		Address:        addr,
		Port:           port,
		AccessLogLevel: "info",
	}

	return New(cfg, zap.NewNop())
}

// freePort returns an available TCP port on localhost that is released
// immediately after discovery. There is a small TOCTOU window, but it is
// acceptable for tests.
func freePort(t *testing.T) uint {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	must.NoError(t, err)

	port := uint(ln.Addr().(*net.TCPAddr).Port)
	must.NoError(t, ln.Close())

	return port
}

func TestServer_Start_bindsSuccessfully(t *testing.T) {
	port := freePort(t)
	s := newTestServer(t, "127.0.0.1", port)

	err := s.Start()
	must.NoError(t, err)

	// Ensure the server is actually listening by opening a TCP connection.
	conn, dialErr := net.DialTimeout("tcp", s.server.Addr, time.Second)
	must.NoError(t, dialErr)
	_ = conn.Close()

	// Clean shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	must.NoError(t, s.Shutdown(ctx))
}

func TestServer_Start_portAlreadyInUse(t *testing.T) {
	// Hold a port so that our Server cannot bind to it.
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	must.NoError(t, err)
	defer helper.DeferedErrorIgnore(blocker.Close)

	port := uint(blocker.Addr().(*net.TCPAddr).Port)
	must.Error(t, newTestServer(t, "127.0.0.1", port).Start())
}
