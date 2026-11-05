package main

import (
	"context"
	"fmt"
	"os"

	"github.com/dncore/lightscraper/internal/app"
	"github.com/dncore/lightscraper/internal/config"
	"github.com/dncore/lightscraper/internal/database"
	"github.com/dncore/lightscraper/internal/logging"
	"github.com/dncore/lightscraper/internal/version"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "lightscraper:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	logger, err := logging.New(cfg.LogLevel, os.Stderr)
	if err != nil {
		return fmt.Errorf("initialize logging: %w", err)
	}
	logger.Info("starting LightScraper", "version", version.Version, "commit", version.Commit)

	if err := cfg.EnsureDataDir(); err != nil {
		return err
	}
	store, err := database.Open(cfg.DatabasePath())
	if err != nil {
		logger.Error("database initialization failed", "error", err)
		return fmt.Errorf("initialize database: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error("close database", "error", err)
		}
	}()

	application, err := app.New(app.Options{
		Store:  store,
		Logger: logger,
		Emitter: func(ctx context.Context, event string, data ...any) {
			runtime.EventsEmit(ctx, event, data...)
		},
	})
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}

	err = wails.Run(&options.App{
		Title:     "LightScraper",
		Width:     960,
		Height:    680,
		MinWidth:  720,
		MinHeight: 520,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 16, G: 19, B: 24, A: 1},
		OnStartup:        application.Startup,
		OnShutdown:       application.Shutdown,
		Bind: []interface{}{
			application,
		},
	})
	if err != nil {
		return fmt.Errorf("run desktop application: %w", err)
	}
	return nil
}
