package app

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/dncore/lightscraper/internal/database"
	"github.com/dncore/lightscraper/internal/version"
)

const (
	// EventHeartbeat is emitted periodically to prove the Go background event
	// path to the frontend. It is deliberately not a task system.
	EventHeartbeat = "backend:heartbeat"

	defaultHeartbeatInterval = 2 * time.Second
	pingTimeout              = 5 * time.Second
)

// Emitter is the small event surface the app needs. main wires it to the Wails
// runtime; tests can use a channel.
type Emitter func(ctx context.Context, event string, data ...any)

// Options are the dependencies needed by App.
type Options struct {
	Store             *database.Store
	Logger            *slog.Logger
	Emitter           Emitter
	HeartbeatInterval time.Duration
}

// App is the Wails-bound application service. Its exported methods form the
// minimal Go-to-frontend API for this task.
type App struct {
	store             *database.Store
	logger            *slog.Logger
	emit              Emitter
	heartbeatInterval time.Duration
	info              AppInfo

	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// AppInfo is the frontend-facing application identity.
type AppInfo struct {
	Name      string
	Version   string
	Commit    string
	BuildDate string
}

// PingResult confirms that the Go backend and SQLite store are reachable.
type PingResult struct {
	Status        string
	Message       string
	Database      string
	SchemaVersion int
	GoVersion     string
}

// Heartbeat is the payload of EventHeartbeat.
type Heartbeat struct {
	Sequence int
	SentAt   string
	Version  string
}

// New constructs the application service after validating its dependencies.
func New(options Options) (*App, error) {
	if options.Store == nil {
		return nil, fmt.Errorf("database store is required")
	}
	if options.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if options.Emitter == nil {
		return nil, fmt.Errorf("event emitter is required")
	}
	if options.HeartbeatInterval < 0 {
		return nil, fmt.Errorf("heartbeat interval cannot be negative")
	}
	if options.HeartbeatInterval == 0 {
		options.HeartbeatInterval = defaultHeartbeatInterval
	}

	current := version.Current()
	return &App{
		store:             options.Store,
		logger:            options.Logger,
		emit:              options.Emitter,
		heartbeatInterval: options.HeartbeatInterval,
		info: AppInfo{
			Name:      current.Name,
			Version:   current.Version,
			Commit:    current.Commit,
			BuildDate: current.BuildDate,
		},
	}, nil
}

// Startup is wired to the Wails OnStartup hook. The Wails context is retained
// only for runtime calls made by the injected emitter.
func (a *App) Startup(parent context.Context) {
	a.mu.Lock()
	if a.cancel != nil {
		a.mu.Unlock()
		a.logger.Warn("startup called more than once; ignoring")
		return
	}
	ctx, cancel := context.WithCancel(parent)
	a.cancel = cancel
	a.mu.Unlock()

	a.logger.Info("application started", "version", a.info.Version, "commit", a.info.Commit)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.runHeartbeat(ctx)
	}()
}

// Shutdown is wired to the Wails OnShutdown hook. It stops the background
// goroutine and closes SQLite before the process exits.
func (a *App) Shutdown(context.Context) {
	a.mu.Lock()
	cancel := a.cancel
	a.cancel = nil
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	a.wg.Wait()

	if err := a.store.Close(); err != nil {
		a.logger.Error("close database", "error", err)
	}
	a.logger.Info("application stopped")
}

// GetAppInfo returns name and build metadata for the window header.
func (a *App) GetAppInfo() AppInfo {
	return a.info
}

// Ping checks the database and returns the current schema version. A non-nil
// error is surfaced to the frontend as a rejected promise. The timeout lives
// here so the frontend cannot pass an arbitrary context through the binding.
func (a *App) Ping() (PingResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()

	if err := a.store.Ping(ctx); err != nil {
		return PingResult{}, fmt.Errorf("database unavailable: %w", err)
	}
	schemaVersion, err := a.store.SchemaVersion(ctx)
	if err != nil {
		return PingResult{}, fmt.Errorf("read schema version: %w", err)
	}

	return PingResult{
		Status:        "ok",
		Message:       "Backend connected",
		Database:      "ready",
		SchemaVersion: schemaVersion,
		GoVersion:     runtime.Version(),
	}, nil
}

func (a *App) runHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(a.heartbeatInterval)
	defer ticker.Stop()

	sequence := 0
	for {
		select {
		case <-ctx.Done():
			a.logger.Debug("background event loop stopped")
			return
		case <-ticker.C:
			sequence++
			a.emit(ctx, EventHeartbeat, Heartbeat{
				Sequence: sequence,
				SentAt:   time.Now().UTC().Format(time.RFC3339),
				Version:  a.info.Version,
			})
		}
	}
}
