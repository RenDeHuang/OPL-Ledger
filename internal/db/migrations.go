package db

import "embed"

// Migrations embeds SQL files from the package-local migrations directory.
//
//go:embed migrations/*.sql
var Migrations embed.FS
