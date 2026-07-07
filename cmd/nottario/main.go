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

	// Embed the zoneinfo database so `TZ=Europe/Madrid` (or any other
	// named zone) resolves correctly inside minimal container images
	// like the alpine base used by the Dockerfile, which do not ship
	// tzdata by default. Without this, time.Local falls back to UTC
	// and the backup goroutine fires at the wrong wall-clock hour.
	// Adds ~450KB to the binary; canonical Go-side fix per the
	// time/tzdata package docs.
	_ "time/tzdata"

	"github.com/neverbot/nottario/internal/arch"
	"github.com/neverbot/nottario/internal/backup"
	"github.com/neverbot/nottario/internal/config"
	"github.com/neverbot/nottario/internal/db"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/notifications"
	"github.com/neverbot/nottario/internal/realtime"
	"github.com/neverbot/nottario/internal/selfupdate"
	"github.com/neverbot/nottario/internal/tasks"
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
		RequiredOrg:  cfg.GithubOrg,
	}

	hub := realtime.New(logger)
	go func() {
		if err := hub.Run(ctx, pool); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("realtime listener stopped", "err", err)
		}
	}()

	// Belt-and-suspenders reconciler: closes feature parents whose
	// children are all done but whose own state never got updated.
	// Live path already does this inside SetState's transaction; this
	// catches anything that drifted while the process was down.
	reconciler := &tasks.RollUpReconciler{Pool: pool, Interval: 60 * time.Second, Logger: logger}
	go func() {
		if err := reconciler.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("rollup reconciler stopped", "err", err)
		}
	}()

	go func() {
		if err := backup.Run(ctx, backup.Config{
			Dir:         cfg.BackupDir,
			DatabaseURL: cfg.DatabaseURL,
			At:          cfg.BackupAt,
			KeepDays:    cfg.BackupKeepDays,
			Logger:      logger.With("subsystem", "backup"),
		}); err != nil {
			logger.Error("backup goroutine exited with error", "err", err)
		}
	}()

	// Arch versioning: configure the per-session idle threshold and
	// start the background ticker that auto-flushes expired sessions
	// into arch_revisions rows. The lock-acquire path in arch writes
	// also evicts expired locks inline, but the ticker is what keeps
	// the revisions log timely when no one is writing.
	arch.SetIdleConfig(arch.IdleConfig{DefaultSeconds: cfg.ArchLockIdleSeconds})
	arch.NewFlushTicker(pool, time.Duration(cfg.ArchTickSeconds)*time.Second,
		logger.With("subsystem", "arch-flush")).Start(ctx)

	// Self-update poller: optional. When disabled the state is nil
	// and the /api/version/status endpoint reports check_enabled=
	// false, so the frontend never shows the "update available"
	// banner.
	var selfUpdateState *selfupdate.State
	if cfg.SelfUpdateEnabled {
		p := selfupdate.New(selfupdate.Config{
			Upstream: cfg.SelfUpdateUpstream,
			Interval: cfg.SelfUpdateInterval,
			Logger:   logger.With("subsystem", "selfupdate"),
			// Broadcast a minimal signal on every observable state
			// transition — the banner re-fetches /api/version/status
			// (which is already admin-gated) instead of receiving the
			// SHAs on the wire.
			Notifier: func() {
				hub.PublishGlobal(realtime.Event{Type: "version_status"})
			},
		})
		selfUpdateState = p.State()
		go p.Start(ctx)
	} else {
		logger.Info("self-update poller disabled (SELF_UPDATE_CHECK_ENABLED=false)")
	}

	notifier := notifications.New(pool, hub,
		logger.With("subsystem", "notifications"),
		cfg.NotificationsEnabled)
	if !cfg.NotificationsEnabled {
		logger.Info("notifications disabled (NOTIFICATIONS_ENABLED=false)")
	}

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: web.NewServer(web.Deps{
			Pool:                 pool,
			Resolver:             resolver,
			OAuthConfig:          oauthCfg,
			Hub:                  hub,
			SelfUpdateState:      selfUpdateState,
			SelfUpdateUpstream:   cfg.SelfUpdateUpstream,
			Notifier:             notifier,
			NotificationsEnabled: cfg.NotificationsEnabled,
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
