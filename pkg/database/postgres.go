package database

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(dsn string) *pgxpool.Pool {
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("unable to create connection pool: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("unable to reach database: %v", err)
	}
	return pool
}
