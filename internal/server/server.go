package server

import (
	"context"
	"log"

	"github.com/dcm-io/dcm/pkg/store"
	kinestore "github.com/dcm-io/dcm/pkg/store/kine"
)

// Server is the DCM control plane server.
type Server struct {
	store store.Store
}

// Config holds server configuration.
type Config struct {
	DataDir string
}

// New creates and starts a new Server.
func New(ctx context.Context, cfg Config) (*Server, error) {
	s, err := kinestore.New(ctx, kinestore.Config{
		DataDir: cfg.DataDir,
	})
	if err != nil {
		return nil, err
	}

	log.Printf("DCM server started (data=%s)", cfg.DataDir)
	return &Server{store: s}, nil
}

// Store returns the underlying store for use by controllers.
func (s *Server) Store() store.Store {
	return s.store
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	log.Println("DCM server stopping")
	return s.store.Close()
}
