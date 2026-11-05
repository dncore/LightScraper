package logging

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// New creates a text logger for the requested level. Callers are responsible
// for keeping secrets and sensitive absolute paths out of log messages.
func New(level string, output io.Writer) (*slog.Logger, error) {
	if output == nil {
		return nil, fmt.Errorf("log output is required")
	}

	var parsed slog.Level
	if err := parsed.UnmarshalText([]byte(strings.ToUpper(strings.TrimSpace(level)))); err != nil {
		return nil, fmt.Errorf("parse log level %q: %w", level, err)
	}

	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: parsed})), nil
}
