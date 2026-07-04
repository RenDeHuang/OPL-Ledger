package db

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"sort"
)

// Migrations embeds SQL files from the package-local migrations directory.
//
//go:embed migrations/*.sql
var Migrations embed.FS

func RunMigrations(ctx context.Context, db *sql.DB) error {
	paths, err := fs.Glob(Migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(paths)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	for _, path := range paths {
		statement, err := Migrations.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(statement)); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}
