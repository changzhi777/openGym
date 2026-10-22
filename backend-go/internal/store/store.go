// Package store — PostgreSQL 连接(pgx v5 pool)。sqlc 生成代码(P3)也落这里。
package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	if url == "" {
		url = "postgres://muhe:muhe@localhost:5432/muhe?sslmode=disable"
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 20
	cfg.MaxConnLifetime = time.Hour
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return pool, pool.Ping(ctx)
}
