package api

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/dcm-io/dcm/pkg/store"
)

// Server is the DCM HTTP API server.
type Server struct {
	httpServer *http.Server
}

// NewServer creates an API server with all routes registered.
func NewServer(addr string, s store.Store) *Server {
	mux := http.NewServeMux()
	RegisterRoutes(mux, s)

	return &Server{
		httpServer: &http.Server{
			Addr:    addr,
			Handler: LoggingMiddleware(mux),
		},
	}
}

// Start begins listening for HTTP requests in a goroutine.
func (s *Server) Start() error {
	log.Printf("API server listening on %s", s.httpServer.Addr)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("API server error: %v", err)
		}
	}()
	return nil
}

// Stop gracefully shuts down the HTTP server.
func (s *Server) Stop(ctx context.Context) error {
	log.Println("API server shutting down")
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
