package redis

import (
	"context"
	"fmt"
	"time"

	"hms/go-backend/internal/config"

	redisdriver "github.com/redis/go-redis/v9"
)

func Connect(ctx context.Context, cfg config.RedisConfig) (*redisdriver.Client, error) {
	options, err := redisdriver.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	client := redisdriver.NewClient(options)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return client, nil
}
