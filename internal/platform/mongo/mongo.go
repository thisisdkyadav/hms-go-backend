package mongo

import (
	"context"
	"fmt"
	"time"

	"hms/go-backend/internal/config"

	mongodriver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func Connect(ctx context.Context, cfg config.MongoConfig) (*mongodriver.Client, *mongodriver.Database, error) {
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client, err := mongodriver.Connect(connectCtx, options.Client().ApplyURI(cfg.URI))
	if err != nil {
		return nil, nil, fmt.Errorf("connect mongo: %w", err)
	}

	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()

	if err := client.Ping(pingCtx, nil); err != nil {
		return nil, nil, fmt.Errorf("ping mongo: %w", err)
	}

	return client, client.Database(cfg.Database), nil
}
