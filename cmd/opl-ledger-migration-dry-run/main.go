package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/RenDeHuang/OPL-Ledger/internal/migration"
)

func main() {
	inputDir := flag.String("input", ".local/migration-dry-run/input", "directory containing local medopl-3 JSON exports")
	outputDir := flag.String("output", ".local/migration-dry-run", "directory for local preview files and migration-report.json")
	flag.Parse()

	report, err := migration.RunDryRun(*inputDir, *outputDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration dry run failed: %v\n", err)
		os.Exit(1)
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode migration report: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(payload))
	if report.Status != "pass" {
		os.Exit(2)
	}
}
