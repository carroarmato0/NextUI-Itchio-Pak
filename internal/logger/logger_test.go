package logger_test

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// captureOutput redirects stdlib log output to a buffer for the test duration.
// Flags are zeroed so assertions don't have to account for the timestamp.
func captureOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
	})
	return &buf
}

// resetLevel restores the logger to INFO after each test.
func resetLevel(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { logger.SetLevel(logger.LevelInfo) })
}

func TestLevelFromString(t *testing.T) {
	cases := []struct {
		input string
		want  logger.Level
	}{
		{"debug", logger.LevelDebug},
		{"DEBUG", logger.LevelDebug},
		{"Debug", logger.LevelDebug},
		{"info", logger.LevelInfo},
		{"INFO", logger.LevelInfo},
		{"", logger.LevelInfo},
		{"verbose", logger.LevelInfo},
		{"warn", logger.LevelInfo}, // "warn" is not a valid input — maps to info
	}
	for _, c := range cases {
		got := logger.LevelFromString(c.input)
		if got != c.want {
			t.Errorf("LevelFromString(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

func TestInfoLevel_SuppressesDebug(t *testing.T) {
	resetLevel(t)
	logger.SetLevel(logger.LevelInfo)
	buf := captureOutput(t)

	logger.Debug("should not appear")
	logger.Info("should appear")

	out := buf.String()
	if strings.Contains(out, "should not appear") {
		t.Error("DEBUG message appeared at INFO level")
	}
	if !strings.Contains(out, "should appear") {
		t.Error("INFO message did not appear at INFO level")
	}
}

func TestDebugLevel_ShowsAll(t *testing.T) {
	resetLevel(t)
	logger.SetLevel(logger.LevelDebug)
	buf := captureOutput(t)

	logger.Debug("debug msg")
	logger.Info("info msg")
	logger.Warn("warn msg")
	logger.Error("error msg")

	out := buf.String()
	for _, want := range []string{"debug msg", "info msg", "warn msg", "error msg"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestLevelTags_AlignedWidth(t *testing.T) {
	resetLevel(t)
	logger.SetLevel(logger.LevelDebug)
	buf := captureOutput(t)

	logger.Debug("d")
	logger.Info("i")
	logger.Warn("w")
	logger.Error("e")

	out := buf.String()
	for _, tag := range []string{"[DEBUG] ", "[INFO]  ", "[WARN]  ", "[ERROR] "} {
		if !strings.Contains(out, tag) {
			t.Errorf("expected tag %q in output:\n%s", tag, out)
		}
	}
}

func TestRegisterSecret_RedactsInOutput(t *testing.T) {
	resetLevel(t)
	logger.SetLevel(logger.LevelInfo)
	buf := captureOutput(t)

	// Use a unique value unlikely to appear in other test output.
	secret := "test-secret-aBcDeF-12345"
	logger.RegisterSecret(secret, "[TEST-SECRET]")
	logger.Info("url=https://example.com/api/%s/game/99", secret)

	out := buf.String()
	if strings.Contains(out, secret) {
		t.Errorf("secret appeared in log output:\n%s", out)
	}
	if !strings.Contains(out, "[TEST-SECRET]") {
		t.Errorf("redaction label not found in output:\n%s", out)
	}
}

func TestRegisterSecret_EmptyValueIsNoop(t *testing.T) {
	resetLevel(t)
	logger.SetLevel(logger.LevelInfo)
	buf := captureOutput(t)

	logger.RegisterSecret("", "[NOOP]")
	logger.Info("plain message no secrets")

	out := buf.String()
	if !strings.Contains(out, "plain message no secrets") {
		t.Errorf("unexpected output:\n%s", out)
	}
}

func TestRegisterSecret_UpdatesExistingLabel(t *testing.T) {
	resetLevel(t)
	logger.SetLevel(logger.LevelInfo)
	buf := captureOutput(t)

	newKey := "test-new-key-xYzAbC-99887"
	logger.RegisterSecret("test-old-key-mNoPqR-11223", "[UPDATE-TEST]")
	logger.RegisterSecret(newKey, "[UPDATE-TEST]") // replaces old entry

	logger.Info("key=%s in message", newKey)

	out := buf.String()
	if strings.Contains(out, newKey) {
		t.Errorf("updated secret still visible in output:\n%s", out)
	}
	if !strings.Contains(out, "[UPDATE-TEST]") {
		t.Errorf("redaction label not found:\n%s", out)
	}
}
