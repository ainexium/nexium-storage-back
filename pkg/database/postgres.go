package database

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(dsn string) *pgxpool.Pool {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		log.Fatalf("invalid database URL: %v", err)
	}
	config.MaxConns = 50
	// pgx v5 ignore ?pgbouncer=true dans l'URL — on force SimpleProtocol
	// pour être compatible PgBouncer en mode transaction (Supabase pooler port 6543).
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		log.Fatalf("unable to create connection pool: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("unable to reach database: %v", err)
	}
	return pool
}
