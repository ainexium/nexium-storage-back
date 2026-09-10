package database

import (
	"context"
	"embed"
	"fmt"
	"log"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

func RunMigrations(db *pgxpool.Pool, fs embed.FS) error {
	ctx := context.Background()

	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       TEXT        PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	rows, err := db.Query(ctx, `SELECT name FROM schema_migrations`)
	if err != nil {
		return err
	}
	applied := map[string]bool{}
	for rows.Next() {
		var name string
		rows.Scan(&name)
		applied[name] = true
	}
	rows.Close()

	entries, err := fs.ReadDir(".")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, e := range entries {
		name := e.Name()
		if applied[name] {
			continue
		}
		sql, err := fs.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err := db.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("migration %s failed: %w", name, err)
		}
		db.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1) ON CONFLICT DO NOTHING`, name)
		log.Printf("[migrate] applied: %s", name)
	}
	return nil
}
