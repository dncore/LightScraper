package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewHonorsLevel(t *testing.T) {
	var output bytes.Buffer
	logger, err := New("info", &output)
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	logger.Debug("hidden debug message")
	logger.Info("visible info message")

	text := output.String()
	if strings.Contains(text, "hidden debug message") {
		t.Fatalf("debug message should be filtered at info level: %s", text)
	}
	if !strings.Contains(text, "visible info message") {
		t.Fatalf("expected info message in output: %s", text)
	}
}

func TestNewRejectsInvalidInput(t *testing.T) {
	if _, err := New("verbose", &bytes.Buffer{}); err == nil {
		t.Fatal("expected invalid level to fail")
	}
	if _, err := New("info", nil); err == nil {
		t.Fatal("expected nil output to fail")
	}
}
