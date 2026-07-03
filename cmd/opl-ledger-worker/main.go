package main

import (
	"log"

	"github.com/RenDeHuang/OPL-Ledger/internal/version"
)

func main() {
	log.Printf("%s worker %s ready; scheduled reconciliation and evidence jobs are outside the baseline scope", version.ServiceName, version.APIVersion)
}
