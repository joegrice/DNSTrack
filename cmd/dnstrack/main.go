package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joe/dnstrack/internal/api"
	"github.com/joe/dnstrack/internal/config"
	"github.com/joe/dnstrack/internal/scheduler"
	"github.com/joe/dnstrack/internal/server"
	"github.com/joe/dnstrack/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	dbPath := flag.String("db", "data/dnstrack.db", "path to SQLite database")
	frontendDir := flag.String("frontend", "web/dist", "path to frontend dist directory")
	flag.Parse()

	// Ensure data directory exists
	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatalf("failed to create data directory: %v", err)
	}

	log.Println("[main] loading config from", *configPath)
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	log.Println("[main] opening database at", *dbPath)
	st, err := store.New(*dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer st.Close()

	sch := scheduler.New(cfg, st)
	if err := sch.Start(); err != nil {
		log.Fatalf("failed to start scheduler: %v", err)
	}
	defer sch.Stop()

	// Run an initial test immediately
	log.Println("[main] running initial test...")
	if err := sch.RunTests(); err != nil {
		log.Printf("[main] initial test error: %v", err)
	}

	handler := api.New(st, sch, cfg)
	srv := server.New(handler, cfg.Server.Port, *frontendDir)

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("server error: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("[main] shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[main] server shutdown error: %v", err)
	}

	sch.Stop()
	st.Close()
}
