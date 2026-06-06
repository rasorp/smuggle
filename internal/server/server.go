package server

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/config"
	"github.com/rasorp/smuggle/internal/log"
	"github.com/rasorp/smuggle/internal/rpc"
	smugglestore "github.com/rasorp/smuggle/internal/store"
)

type Server struct {
	cfg      *config.ServerConfig
	logger   *log.Logger
	store    smugglestore.BackingStore
	handlers *rpc.Handlers

	// knownNetworks is used by the network reaper to detect deleted networks
	// across runs. It is populated on the first reaper run and updated on every
	// subsequent run. Access is confined to the reaper goroutine, so no mutex is
	// required.
	knownNetworks map[string]struct{}

	rpcServer *rpc.Server

	// shutdownCh is used to signal to all server processes that the agent is
	// shutting down. All long-running processes should monitor this channel and
	// use the shutdownGroup wait group to ensure the agent does not exit before
	// they have completed.
	shutdownCh    chan struct{}
	shutdownGroup sync.WaitGroup
}

type ServerReq struct {
	Config *config.ServerConfig
	Logger *log.Logger
	Store  smugglestore.BackingStore
}

func New(req *ServerReq) (*Server, error) {
	return &Server{
		cfg:        req.Config,
		logger:     req.Logger.Named(log.ComponentNameServer),
		store:      req.Store,
		shutdownCh: make(chan struct{}),
	}, nil
}

func (s *Server) Start() error {
	s.logger.Info("starting server")

	handlers, err := rpc.NewHandlers(&rpc.HandlerReq{
		Store:          s.store,
		WriteRateLimit: s.cfg.RPC.WriteRateLimit,
		WriteBurst:     s.cfg.RPC.WriteBurst,
		StopCh:         s.shutdownCh,
		Logger:         s.logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create RPC handler: %w", err)
	}
	s.handlers = handlers

	rpcSrv, err := rpc.NewServer(s.cfg.RPC.Addr(), handlers, s.logger, s.cfg.RPC.AccessLogLevel)
	if err != nil {
		return fmt.Errorf("failed to create RPC server: %w", err)
	}
	s.rpcServer = rpcSrv

	s.shutdownGroup.Go(func() {
		s.rpcServer.Serve()
	})

	go s.startNetworkReaper()
	return nil
}

func (s *Server) Stop() error {
	s.logger.Info("shutting down server")

	// Close the RPC listener first so the Serve goroutine exits and new client
	// connections are rejected before we signal shutdown to other goroutines.
	if s.rpcServer != nil {
		if err := s.rpcServer.Stop(); err != nil {
			s.logger.Error("error stopping RPC server", zap.Error(err))
		}
	}

	close(s.shutdownCh)

	// In order to avoid blocking forever is the shutdown groups do not
	// terminate correctly, we use a timer to enforce a timeout. In order to do
	// this, we use a channel that will unblock once the wait group is done.
	waitFinishedCh := make(chan struct{})

	go func() {
		s.shutdownGroup.Wait()
		close(waitFinishedCh)
	}()

	t := time.NewTimer(10 * time.Second)
	defer t.Stop()

	// Wait for either the wait group to finish or the timer to expire.
	// Returning an error provides operator feedback that something is not right
	// during shutdown.
	select {
	case <-t.C:
		return errors.New("timeout waiting for shutdown")
	case <-waitFinishedCh:
	}
	return nil
}
