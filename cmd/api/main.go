package main

import (
	"context"
	"log/slog"
	"os"

	"hms/go-backend/internal/app"
)

func main() {
	application, err := app.New(context.Background())
	if err != nil {
		slog.Error("failed to initialize application", "error", err)
		os.Exit(1)
	}

	if err := application.Run(context.Background()); err != nil {
		slog.Error("application exited with error", "error", err)
		os.Exit(1)
	}
}
