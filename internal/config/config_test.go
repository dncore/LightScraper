package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv(EnvDataDir, "")
	t.Setenv(EnvLogLevel, "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}

	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("user config dir: %v", err)
	}
	expected, err := filepath.Abs(filepath.Join(base, AppName))
	if err != nil {
		t.Fatalf("absolute expected path: %v", err)
	}
	if cfg.DataDir != expected {
		t.Fatalf("expected data dir %q, got %q", expected, cfg.DataDir)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("expected info log level, got %q", cfg.LogLevel)
	}
}

func TestLoadOverrides(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "portable")
	t.Setenv(EnvDataDir, dataDir)
	t.Setenv(EnvLogLevel, "DEBUG")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load overrides: %v", err)
	}
	expected, err := filepath.Abs(dataDir)
	if err != nil {
		t.Fatalf("absolute data dir: %v", err)
	}
	if cfg.DataDir != expected {
		t.Fatalf("expected data dir %q, got %q", expected, cfg.DataDir)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("expected debug log level, got %q", cfg.LogLevel)
	}
}

func TestLoadRejectsInvalidLogLevel(t *testing.T) {
	t.Setenv(EnvLogLevel, "verbose")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid log level to be rejected")
	}
}

func TestDatabasePathAndEnsureDataDir(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "nested", "data")
	cfg := Config{DataDir: dataDir}
	if got, want := cfg.DatabasePath(), filepath.Join(dataDir, "lightscraper.db"); got != want {
		t.Fatalf("expected database path %q, got %q", want, got)
	}
	if err := cfg.EnsureDataDir(); err != nil {
		t.Fatalf("ensure data dir: %v", err)
	}
	info, err := os.Stat(dataDir)
	if err != nil {
		t.Fatalf("stat data dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", dataDir)
	}
}
