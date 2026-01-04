package server

import (
	"errors"
	"sync"
	"time"

	"github.com/rasorp/smuggle/internal/config"
	"github.com/rasorp/smuggle/internal/log"
	"github.com/rasorp/smuggle/internal/types"
)

type Server struct {
	cfg    *config.ServerConfig
	logger *log.Logger

	// store
	store types.Store

	// shtutdownCh is used to signal to all server processes that the agent is
	// shutting down. All long-running processes should monitor this channel and
	// use the shutdownGroup wait group to ensure the agent does not exit before
	// they have completed.
	shutdownCh    chan struct{}
	shutdownGroup sync.WaitGroup
}

type ServerReq struct {
	Config *config.ServerConfig
	Logger *log.Logger
	Store  types.Store
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
	go s.startNetworkReaper()
	return nil
}

func (s *Server) Stop() error {
	s.logger.Info("shutting down server")

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
