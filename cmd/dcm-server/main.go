package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dcm-io/dcm/internal/server"
)

func main() {
	dataDir := flag.String("data-dir", "/tmp/dcm", "Directory for SQLite database and unix socket")
	listenAddr := flag.String("listen", ":8080", "HTTP listen address")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := server.New(ctx, server.Config{
		DataDir:    *dataDir,
		ListenAddr: *listenAddr,
	})
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %s, shutting down", sig)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Stop(shutdownCtx); err != nil {
		log.Fatalf("Failed to stop server: %v", err)
	}
}
