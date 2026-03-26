// Command caster runs the NTRIP Caster server.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ntrip-caster/internal/account"
	"ntrip-caster/internal/api"
	"ntrip-caster/internal/caster"
	"ntrip-caster/internal/config"
	"ntrip-caster/internal/database"
	"ntrip-caster/internal/mountpoint"
	"ntrip-caster/pkg/logger"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	logger.Init(slog.LevelInfo)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		slog.Error("failed to open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	acctSvc := account.NewService(db)

	if err := acctSvc.EnsureAdmin("admin", "admin"); err != nil {
		slog.Error("failed to ensure admin user", "err", err)
		os.Exit(1)
	}

	mgr := mountpoint.NewManager()
	if err := syncMountpoints(acctSvc, mgr, cfg); err != nil {
		slog.Error("failed to sync mountpoints", "err", err)
		os.Exit(1)
	}

	ntripServer := caster.NewServer(cfg, mgr, acctSvc)
	adminServer := api.NewHTTPServer(cfg, acctSvc, mgr)

	errCh := make(chan error, 2)

	go func() {
		errCh <- ntripServer.ListenAndServe()
	}()
	go func() {
		errCh <- adminServer.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		slog.Info("received signal, shutting down", "signal", sig)
	case err := <-errCh:
		if err != nil {
			slog.Error("server error", "err", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.Info("shutting down NTRIP server")
	if err := ntripServer.Shutdown(shutdownCtx); err != nil {
		slog.Warn("NTRIP shutdown error", "err", err)
	}

	slog.Info("shutting down Admin API server")
	if err := adminServer.Shutdown(shutdownCtx); err != nil {
		slog.Warn("Admin API shutdown error", "err", err)
	}

	slog.Info("shutdown complete")
}

// syncMountpoints loads mountpoint records from the database and creates
// them in the in-memory manager.
func syncMountpoints(acctSvc *account.Service, mgr *mountpoint.Manager, cfg *config.Config) error {
	rows, err := acctSvc.ListMountPointRows()
	if err != nil {
		return err
	}
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		wq := cfg.MountpointDefaults.WriteQueue
		if row.WriteQueue != nil {
			wq = *row.WriteQueue
		}
		wt := cfg.MountpointDefaults.WriteTimeout
		if row.WriteTimeoutMs != nil {
			wt = time.Duration(*row.WriteTimeoutMs) * time.Millisecond
		}
		if _, err := mgr.Create(row.Name, row.Description, row.Format, wq, wt, row.MaxClients); err != nil {
			slog.Warn("skip mountpoint", "name", row.Name, "err", err)
		}
	}
	return nil
}
