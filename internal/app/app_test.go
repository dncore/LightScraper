package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/dncore/lightscraper/internal/database"
)

func newTestApp(t *testing.T, emitter Emitter, interval time.Duration) (*App, *database.Store) {
	t.Helper()

	store, err := database.Open(t.TempDir() + "/lightscraper.db")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application, err := New(Options{
		Store:             store,
		Logger:            logger,
		Emitter:           emitter,
		HeartbeatInterval: interval,
	})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	return application, store
}

func TestGetAppInfo(t *testing.T) {
	application, _ := newTestApp(t, func(context.Context, string, ...any) {}, 0)

	info := application.GetAppInfo()
	if info.Name != "LightScraper" {
		t.Fatalf("expected app name LightScraper, got %q", info.Name)
	}
	if info.Version == "" {
		t.Fatal("expected a non-empty version")
	}
}

func TestPingChecksDatabase(t *testing.T) {
	application, _ := newTestApp(t, func(context.Context, string, ...any) {}, 0)

	result, err := application.Ping()
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	if result.Status != "ok" {
		t.Fatalf("expected ok status, got %q", result.Status)
	}
	if result.SchemaVersion != 1 {
		t.Fatalf("expected schema version 1, got %d", result.SchemaVersion)
	}
	if result.GoVersion == "" {
		t.Fatal("expected Go version")
	}
}

func TestHeartbeatAndShutdown(t *testing.T) {
	events := make(chan Heartbeat, 1)
	application, _ := newTestApp(t, func(_ context.Context, event string, data ...any) {
		if event != EventHeartbeat {
			t.Errorf("unexpected event %q", event)
			return
		}
		payload, ok := data[0].(Heartbeat)
		if !ok {
			t.Errorf("unexpected heartbeat payload %T", data[0])
			return
		}
		select {
		case events <- payload:
		default:
		}
	}, 10*time.Millisecond)

	application.Startup(context.Background())
	t.Cleanup(func() {
		application.Shutdown(context.Background())
	})

	select {
	case heartbeat := <-events:
		if heartbeat.Sequence < 1 {
			t.Fatalf("expected positive sequence, got %d", heartbeat.Sequence)
		}
		if heartbeat.Version == "" {
			t.Fatal("expected heartbeat version")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for background heartbeat")
	}

	application.Shutdown(context.Background())
}

func TestNewRejectsMissingDependencies(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("expected missing dependencies to be rejected")
	}
}
