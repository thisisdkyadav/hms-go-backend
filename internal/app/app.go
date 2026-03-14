package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	authmodule "hms/go-backend/internal/modules/auth"
	"hms/go-backend/internal/config"
	mongoplatform "hms/go-backend/internal/platform/mongo"
	redisplatform "hms/go-backend/internal/platform/redis"
	"hms/go-backend/internal/shared/email"
	"hms/go-backend/internal/shared/httpx"
	"hms/go-backend/internal/shared/session"
)

type App struct {
	cfg         config.Config
	logger      *slog.Logger
	httpServer  *http.Server
	mongoCloser func(context.Context) error
	redisCloser func() error
}

func New(ctx context.Context) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	logger := newLogger(cfg.App.Env)

	mongoClient, database, err := mongoplatform.Connect(ctx, cfg.Mongo)
	if err != nil {
		return nil, err
	}

	redisClient, err := redisplatform.Connect(ctx, cfg.Redis)
	if err != nil {
		return nil, err
	}

	emailSender := email.NewSMTP(cfg.SMTP, cfg.FrontendURL, logger)
	sessionManager := session.NewManager(redisClient, cfg.Session)

	authRepository := authmodule.NewRepository(database)
	authService := authmodule.NewService(cfg, logger, authRepository, sessionManager, emailSender)
	authHandler := authmodule.NewHandler(logger, authService, sessionManager)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteSuccess(w, http.StatusOK, map[string]string{"service": "hms-go-backend"}, "")
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteSuccess(w, http.StatusOK, map[string]string{"status": "ok"}, "")
	})
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteSuccess(w, http.StatusOK, map[string]string{"status": "ok"}, "")
	})
	authHandler.Register(mux)

	handler := httpx.Chain(
		mux,
		httpx.CORS(cfg.AllowedOrigins),
		httpx.RequestLogger(logger),
		httpx.Recover(logger),
	)

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.App.Port),
		Handler:           handler,
		ReadHeaderTimeout: cfg.App.ReadTimeout,
		WriteTimeout:      cfg.App.WriteTimeout,
	}

	return &App{
		cfg:        cfg,
		logger:     logger,
		httpServer: server,
		mongoCloser: func(closeCtx context.Context) error {
			return mongoClient.Disconnect(closeCtx)
		},
		redisCloser: redisClient.Close,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	serverErrors := make(chan error, 1)

	go func() {
		a.logger.Info("starting HTTP server", "addr", a.httpServer.Addr)
		if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
		close(serverErrors)
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	select {
	case err := <-serverErrors:
		if err != nil {
			return err
		}
	case <-signals:
		a.logger.Info("shutdown signal received")
	case <-ctx.Done():
		a.logger.Info("context cancelled; shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.App.ShutdownTimeout)
	defer cancel()

	if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}

	if a.mongoCloser != nil {
		if err := a.mongoCloser(shutdownCtx); err != nil {
			return fmt.Errorf("disconnect mongo: %w", err)
		}
	}

	if a.redisCloser != nil {
		if err := a.redisCloser(); err != nil {
			return fmt.Errorf("close redis: %w", err)
		}
	}

	return nil
}

func newLogger(env string) *slog.Logger {
	level := slog.LevelInfo
	if env == "development" {
		level = slog.LevelDebug
	}

	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}
