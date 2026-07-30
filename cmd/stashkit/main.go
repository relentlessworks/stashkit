package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/relentlessworks/stashkit/internal/api"
	"github.com/relentlessworks/stashkit/internal/auth"
	"github.com/relentlessworks/stashkit/internal/config"
	"github.com/relentlessworks/stashkit/internal/store"
)

func main() {
	cfg := config.Load()

	// Initialize store
	st, err := store.New(cfg.DataDir)
	if err != nil {
		log.Fatalf("failed to initialize store: %v", err)
	}

	// Initialize auth manager
	am := auth.NewManager(cfg.Secret, auth.SMTPConfig{
		Host: cfg.SMTPHost,
		Port: cfg.SMTPPort,
		User: cfg.SMTPUser,
		Pass: cfg.SMTPPass,
		From: cfg.SMTPFrom,
	})

	// Initialize API server
	server := api.NewServer(st, am)

	// Start HTTP server
	httpServer := &http.Server{
		Addr:    cfg.Addr,
		Handler: server.Routes(),
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		fmt.Fprintln(os.Stderr, "\n[stashkit] shutting down...")
		httpServer.Close()
	}()

	fmt.Fprintf(os.Stderr, "[stashkit] listening on %s (data: %s)\n", cfg.Addr, cfg.DataDir)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
