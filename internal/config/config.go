// Package config đọc cấu hình từ biến môi trường, dùng chung cho cmd/server và cmd/feed.
package config

import (
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

func Env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// Redis đọc REDIS_URL (mặc định redis://localhost:6379/0).
func Redis() (*redis.Client, error) {
	opt, err := redis.ParseURL(Env("REDIS_URL", "redis://localhost:6379/0"))
	if err != nil {
		return nil, err
	}
	opt.PoolSize = 20
	opt.ReadTimeout = 3 * time.Second
	return redis.NewClient(opt), nil
}

func FeedInterval() time.Duration {
	ms, err := strconv.Atoi(Env("FEED_INTERVAL_MS", "250"))
	if err != nil || ms < 20 {
		ms = 250
	}
	return time.Duration(ms) * time.Millisecond
}
