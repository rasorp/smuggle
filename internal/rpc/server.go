package rpc

import (
	"fmt"
	"net"
	netrpc "net/rpc"

	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/log"
)

// Server wraps a net.Listener and a registered net/rpc server, providing
// lifecycle management for the Smuggle RPC endpoint.
type Server struct {
	listener net.Listener
	server   *netrpc.Server
	logger   *zap.Logger
}

// NewServer creates a Server bound to addr, with NetworkHandler
// registered under NetworkService and SubnetHandler under SubnetService.
func NewServer(addr string, handlers *Handlers, logger *zap.Logger, accessLogLevel string) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to bind RPC listener on %s: %w", addr, err)
	}

	rpcLogger := logger.Named(log.ComponentNameRPC)
	srv := netrpc.NewServer()

	if err := srv.RegisterName(NetworkService, newLoggedNetworkHandler(handlers.Network, rpcLogger, accessLogLevel)); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("failed to register Network RPC handler: %w", err)
	}

	if err := srv.RegisterName(SubnetService, newLoggedSubnetHandler(handlers.Subnet, rpcLogger, accessLogLevel)); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("failed to register Subnet RPC handler: %w", err)
	}

	return &Server{
		listener: ln,
		server:   srv,
		logger:   rpcLogger,
	}, nil
}

// Addr returns the address the server is listening on.
func (s *Server) Addr() string { return s.listener.Addr().String() }

// Serve starts accepting connections. It blocks until the listener is closed
// and is intended to be run in a dedicated goroutine.
func (s *Server) Serve() {
	s.logger.Info("RPC server listening", zap.String("address", s.Addr()))
	s.server.Accept(s.listener)
}

// Stop closes the listener, causing Serve to return.
func (s *Server) Stop() error { return s.listener.Close() }
