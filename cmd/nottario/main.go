// Command nottario is the project's only binary. It boots the
// HTTP server, opens the Postgres connection pool, runs pending
// migrations and serves the web UI, REST API and MCP endpoint.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/neverbot/nottario/internal/config"
	"github.com/neverbot/nottario/internal/db"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/version"
	"github.com/neverbot/nottario/internal/web"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	logger.Info("starting nottario", "version", version.String())

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db open", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		logger.Error("db migrate", "err", err)
		os.Exit(1)
	}
	logger.Info("migrations applied")

	cookieSecure := strings.HasPrefix(cfg.PublicURL, "https://")
	resolver := identity.NewResolver(pool, cfg.SessionKey, cookieSecure)
	oauthCfg := identity.OAuthConfig{
		ClientID:     cfg.GithubClientID,
		ClientSecret: cfg.GithubClientSecret,
		PublicURL:    cfg.PublicURL,
		SessionKey:   cfg.SessionKey,
		CookieSecure: cookieSecure,
	}

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: web.NewServer(web.Deps{
			Pool:        pool,
			Resolver:    resolver,
			OAuthConfig: oauthCfg,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.HTTPAddr, "public_url", cfg.PublicURL)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown requested")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "err", err)
	}
	logger.Info("bye")
}
