package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"

	"github.com/RenDeHuang/OPL-Ledger/internal/api"
	"github.com/RenDeHuang/OPL-Ledger/internal/db"
	"github.com/RenDeHuang/OPL-Ledger/internal/ledger"
	_ "github.com/lib/pq"
)

func main() {
	addr := ":8788"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}
	store := newStore()
	handler := api.NewServer(store)
	log.Printf("opl-ledger-api listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}

func newStore() ledger.Store {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return ledger.NewMemoryStore()
	}
	database, err := sql.Open("postgres", databaseURL)
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	if err := database.PingContext(context.Background()); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}
	if err := db.RunMigrations(context.Background(), database); err != nil {
		log.Fatalf("run postgres migrations: %v", err)
	}
	return ledger.NewPostgresStore(database)
}
