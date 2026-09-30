package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"indico_be/app/http/controllers"
	"indico_be/app/repositories"
	"indico_be/app/routes"
	"indico_be/app/services"
)

func main() {
	// --- bootstrap ---
	databasePath := os.Getenv("DATABASE_PATH")
	if databasePath == "" {
		databasePath = "data/indico.db"
	}
	if databasePath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(databasePath), 0755); err != nil {
			log.Fatalf("create database directory: %v", err)
		}
	}
	repo, err := repositories.NewInventoryRepository(databasePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer repo.Close()

	// SEED_ITEM="item_4021:100,item_9001:50"
	if seeds := os.Getenv("SEED_ITEMS"); seeds != "" {
		for _, s := range strings.Split(seeds, ",") {
			var id string
			var n int
			if parts := strings.SplitN(s, ":", 2); len(parts) == 2 {
				id = parts[0]
				fmt.Sscanf(parts[1], "%d", &n)
			}
			if id != "" && n > 0 {
				if err := repo.Seed(id, n); err != nil {
					log.Fatalf("seed %s: %v", id, err)
				}
			}
		}
	} else if err := repo.Seed("item_4021", 100); err != nil {
		log.Fatalf("seed item_4021: %v", err)
	}

	svc := services.NewInventoryService(repo)
	ic := controllers.NewInventoryController(svc)

	mux := http.NewServeMux()
	routes.Register(mux, ic)

	srv := &http.Server{
		Addr:              ":8085",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// --- background expiry reaper (like a scheduled job) ---
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				svc.CleanupExpired()
			}
		}
	}()

	// --- run + graceful shutdown ---
	go func() {
		log.Println("indico_be listening on :8085")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down gracefully...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("forced shutdown: %v", err)
	}
	log.Println("bye")
}
