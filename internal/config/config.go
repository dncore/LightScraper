package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// AppName is used for the per-user data directory.
	AppName = "LightScraper"

	// EnvDataDir overrides the per-user data directory. It is intended for
	// development, tests, and portable installations.
	EnvDataDir = "LIGHTSCRAPER_DATA_DIR"
	// EnvLogLevel overrides the default log level.
	EnvLogLevel = "LIGHTSCRAPER_LOG_LEVEL"

	defaultLogLevel = "info"
)

// Config contains the small set of runtime settings needed by the skeleton.
// It intentionally does not contain secrets. Additional configuration should
// be added only when a task requires it.
type Config struct {
	DataDir  string
	LogLevel string
}

// Load resolves configuration from environment variables and platform
// defaults. The data directory is created separately by EnsureDataDir so that
// loading configuration has no filesystem side effects.
func Load() (Config, error) {
	dataDir := strings.TrimSpace(os.Getenv(EnvDataDir))
	if dataDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return Config{}, fmt.Errorf("resolve user config directory: %w", err)
		}
		dataDir = filepath.Join(base, AppName)
	}

	absoluteDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve data directory: %w", err)
	}

	logLevel := strings.ToLower(strings.TrimSpace(os.Getenv(EnvLogLevel)))
	if logLevel == "" {
		logLevel = defaultLogLevel
	}
	switch logLevel {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("invalid %s value %q: want debug, info, warn, or error", EnvLogLevel, logLevel)
	}

	return Config{
		DataDir:  absoluteDataDir,
		LogLevel: logLevel,
	}, nil
}

// DatabasePath returns the SQLite database location inside DataDir.
func (c Config) DatabasePath() string {
	return filepath.Join(c.DataDir, "lightscraper.db")
}

// EnsureDataDir creates the per-user data directory with owner-only
// permissions. Existing directories are left unchanged.
func (c Config) EnsureDataDir() error {
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	return nil
}
