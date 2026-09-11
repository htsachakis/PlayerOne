package logging

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// deadHandle behaves like os.Stderr in a windowsgui build: every write fails.
type deadHandle struct{}

func (deadHandle) Write([]byte) (int, error) { return 0, errors.New("the handle is invalid") }

// The log file is what anyone asking "send me your log" is asking for, and it
// is written by the copies that have no console at all.
func TestTheLogFileSurvivesAConsoleThatIsNotThere(t *testing.T) {
	dir := t.TempDir()

	stderr := os.Stderr
	broken, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("opening %s: %v", os.DevNull, err)
	}
	os.Stderr = broken // read-only: writes to it fail, as they do with no console
	t.Cleanup(func() {
		os.Stderr = stderr
		_ = broken.Close()
	})

	l := NewWithFile(LevelInfo, dir, "playerone")
	l.Info("app: PlayerOne starting")
	l.Warn("app: something worth knowing")
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	written, err := os.ReadFile(filepath.Join(dir, "playerone.log"))
	if err != nil {
		t.Fatalf("reading the log: %v", err)
	}
	for _, want := range []string{"PlayerOne starting", "something worth knowing"} {
		if !strings.Contains(string(written), want) {
			t.Errorf("the log does not contain %q; it holds %q", want, written)
		}
	}
}

func TestOptionalSwallowsItsWritersFailures(t *testing.T) {
	n, err := optional{deadHandle{}}.Write([]byte("hello"))
	if err != nil {
		t.Errorf("Write returned %v, want nil", err)
	}
	if n != 5 {
		t.Errorf("Write reported %d bytes, want 5", n)
	}
}

func TestLevelsBelowTheMinimumAreDropped(t *testing.T) {
	dir := t.TempDir()

	l := NewWithFile(LevelWarn, dir, "playerone")
	l.Debug("app: chatter")
	l.Info("app: chatter")
	l.Error("app: a real problem")
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	written, err := os.ReadFile(filepath.Join(dir, "playerone.log"))
	if err != nil {
		t.Fatalf("reading the log: %v", err)
	}
	if strings.Contains(string(written), "chatter") {
		t.Errorf("records below the minimum were written: %q", written)
	}
	if !strings.Contains(string(written), "a real problem") {
		t.Errorf("the log is missing the record that was above the minimum: %q", written)
	}
}
