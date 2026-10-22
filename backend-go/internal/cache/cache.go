// Package cache — Redis 客户端。职责边界(方案 §Redis):会话撤销/限流/热点缓存,
// 不做存储 —— Redis 挂掉只损失最近一分钟计数,一切真相在 PG。
package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

func Open(ctx context.Context, addr string) (*redis.Client, error) {
	if addr == "" {
		addr = "localhost:6379"
	}
	c := redis.NewClient(&redis.Options{Addr: addr, DialTimeout: 3 * time.Second})
	return c, c.Ping(ctx).Err()
}
