package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"campusclaw/internal/auth"
	"campusclaw/internal/config"
	"campusclaw/internal/db"
	"campusclaw/internal/httpapi"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := os.MkdirAll(cfg.UploadDir, 0o750); err != nil {
		log.Fatalf("upload dir: %v", err)
	}
	conn, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := db.Wait(ctx, conn); err != nil {
		log.Fatalf("database: %v", err)
	}
	if err := db.Migrate(ctx, conn); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := db.Seed(ctx, conn, cfg); err != nil {
		log.Fatalf("seed: %v", err)
	}
	auth.DummyHash()

	api := httpapi.New(cfg, conn)
	if err := backfillIndex(ctx, api); err != nil {
		log.Printf("knowledge index backfill: %v", err)
	}

	srv := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("listening on %s", cfg.APIAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func backfillIndex(ctx context.Context, api *httpapi.Server) error {
	var last error
	for i := 0; i < 20; i++ {
		last = api.Backfill(ctx)
		if last == nil {
			return nil
		}
		log.Printf("waiting to index knowledge chunks (%d/20): %v", i+1, last)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return last
}
