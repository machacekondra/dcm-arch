package server

import (
	"context"
	"log"

	"github.com/dcm-io/dcm/pkg/api"
	"github.com/dcm-io/dcm/pkg/store"
	kinestore "github.com/dcm-io/dcm/pkg/store/kine"
)

// Server is the DCM control plane server.
type Server struct {
	store     store.Store
	apiServer *api.Server
}

// Config holds server configuration.
type Config struct {
	DataDir    string
	ListenAddr string
}

// New creates and starts a new Server.
func New(ctx context.Context, cfg Config) (*Server, error) {
	s, err := kinestore.New(ctx, kinestore.Config{
		DataDir: cfg.DataDir,
	})
	if err != nil {
		return nil, err
	}

	apiServer := api.NewServer(cfg.ListenAddr, s)
	if err := apiServer.Start(); err != nil {
		s.Close()
		return nil, err
	}

	log.Printf("DCM server started (data=%s, listen=%s)", cfg.DataDir, cfg.ListenAddr)
	return &Server{store: s, apiServer: apiServer}, nil
}

// Store returns the underlying store for use by controllers.
func (s *Server) Store() store.Store {
	return s.store
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	log.Println("DCM server stopping")
	if err := s.apiServer.Stop(ctx); err != nil {
		log.Printf("API server stop error: %v", err)
	}
	return s.store.Close()
}
