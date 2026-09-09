// Package logging provides the application's leveled logger.
//
// Two rules shape this package: playback-position updates fire several times a
// second and must never reach a normal log, and mpv's own chatter is worth
// keeping but only at DEBUG.
package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Level is the severity of a log record.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "?"
	}
}

// ParseLevel maps a level name to a Level, defaulting to INFO.
func ParseLevel(s string) Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARN", "WARNING":
		return LevelWarn
	case "ERROR":
		return LevelError
	default:
		return LevelInfo
	}
}

// Logger writes leveled records to one or more destinations.
type Logger struct {
	mu    sync.Mutex
	min   Level
	out   *log.Logger
	files []io.Closer
}

// New builds a logger writing to stderr at the given minimum level.
func New(min Level) *Logger {
	return &Logger{
		min: min,
		out: log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds),
	}
}

// NewWithFile writes to both stderr and dir/<name>.log. A file that cannot be
// opened is not fatal — logging degrades to stderr only, because losing the log
// file is never a reason to refuse to start.
func NewWithFile(min Level, dir, name string) *Logger {
	l := New(min)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		l.Warn("logging: cannot create log directory %s: %v", dir, err)
		return l
	}

	path := filepath.Join(dir, name+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		l.Warn("logging: cannot open log file %s: %v", path, err)
		return l
	}

	l.files = append(l.files, f)
	l.out = log.New(io.MultiWriter(os.Stderr, f), "", log.LstdFlags|log.Lmicroseconds)
	l.Info("logging: writing to %s", path)
	return l
}

// SetLevel changes the minimum level at runtime.
func (l *Logger) SetLevel(min Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.min = min
}

// Enabled reports whether records at lvl would be written. Callers use it to
// skip building expensive messages that would only be discarded.
func (l *Logger) Enabled(lvl Level) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return lvl >= l.min
}

func (l *Logger) logf(lvl Level, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if lvl < l.min {
		return
	}
	l.out.Printf("%-5s %s", lvl, fmt.Sprintf(format, args...))
}

func (l *Logger) Debug(format string, args ...any) { l.logf(LevelDebug, format, args...) }
func (l *Logger) Info(format string, args ...any)  { l.logf(LevelInfo, format, args...) }
func (l *Logger) Warn(format string, args ...any)  { l.logf(LevelWarn, format, args...) }
func (l *Logger) Error(format string, args ...any) { l.logf(LevelError, format, args...) }

// Close releases the log file, if any.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var firstErr error
	for _, c := range l.files {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	l.files = nil
	return firstErr
}
