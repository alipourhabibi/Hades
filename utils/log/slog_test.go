package log

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alipourhabibi/Hades/config"
)

// TestSlogHonoursTheDocumentedKeys covers the keys config/dev.yaml declares
// while selecting engine: slog.
//
// The handler used to read Level, AddSource, Output and Format and ignore the
// rest, so a file asking for messageKey: message and levelFormat: lowercase got
// "msg" and "INFO". Anything parsing the output saw different fields depending
// on an unrelated setting.
func TestSlogHonoursTheDocumentedKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")

	logger, err := NewWithConfig(loggerConfigWithKeys(path))
	if err != nil {
		t.Fatalf("NewWithConfig: %v", err)
	}
	logger.Info("hello")
	if err := logger.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var entry map[string]any
	if err := json.Unmarshal(raw, &entry); err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}

	if _, ok := entry["message"]; !ok {
		t.Errorf("messageKey ignored: got keys %v", keysOf(entry))
	}
	if _, ok := entry["msg"]; ok {
		t.Errorf("the default message key is still present: %v", keysOf(entry))
	}
	if got := entry["severity"]; got != "info" {
		t.Errorf("levelKey/levelFormat ignored: severity = %v, want \"info\"", got)
	}
	if _, ok := entry["ts"]; !ok {
		t.Errorf("timeKey ignored: got keys %v", keysOf(entry))
	}
}

func loggerConfigWithKeys(path string) config.Logger {
	c := config.Logger{
		Level:  "info",
		Format: JsonFormat,
		Output: path,
	}
	c.MessageKey = "message"
	c.LevelKey = "severity"
	c.LevelFormat = "lowercase"
	c.TimeKey = "ts"
	c.TimeFormat = "RFC3339"
	return c
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
