package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/dcm-io/dcm/internal/server"
)

func main() {
	dataDir := flag.String("data-dir", "/tmp/dcm", "Directory for SQLite database and unix socket")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := server.New(ctx, server.Config{
		DataDir: *dataDir,
	})
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %s, shutting down", sig)

	if err := srv.Stop(); err != nil {
		log.Fatalf("Failed to stop server: %v", err)
	}
}
