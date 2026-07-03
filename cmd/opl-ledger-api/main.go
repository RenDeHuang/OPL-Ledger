package main

import (
	"log"
	"net/http"
	"os"

	"github.com/RenDeHuang/OPL-Ledger/internal/api"
	"github.com/RenDeHuang/OPL-Ledger/internal/ledger"
)

func main() {
	addr := ":8788"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}
	handler := api.NewServer(ledger.NewMemoryStore())
	log.Printf("opl-ledger-api listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
