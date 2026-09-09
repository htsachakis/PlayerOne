// Package history remembers which files have been watched and where playback
// stopped.
package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"playerone/internal/appdir"
)

// FileName is the history file's name inside the config directory.
const FileName = "history.json"

// MaxEntries caps the recent-files list. Twenty is enough to find last week's
// tutorial and small enough that the menu stays scannable.
const MaxEntries = 20

// Resume thresholds.
const (
	// MinResumePosition is how far in playback must have reached before a
	// position is worth remembering. Below this the viewer has effectively not
	// started.
	MinResumePosition = 10 * time.Second

	// MinRemainingForResume is how much must be left for a position to be worth
	// offering. Inside this window the video is effectively finished, and
	// resuming would drop the viewer onto the closing credits.
	MinRemainingForResume = 2 * time.Minute
)

// Entry is one watched file.
type Entry struct {
	Path     string  `json:"path"`
	Filename string  `json:"filename"`
	Title    string  `json:"title"`
	Position float64 `json:"position"`
	Duration float64 `json:"duration"`

	// UpdatedAt orders the recent-files list.
	UpdatedAt time.Time `json:"updatedAt"`

	// Exists is filled in when the list is read, not stored. A file on an
	// unplugged drive should be shown greyed out rather than silently dropped.
	Exists bool `json:"exists"`
}

// ShouldStore reports whether a position is worth remembering.
//
// Duration is allowed to be zero (still loading), in which case only the
// start-of-file rule applies.
func ShouldStore(position, duration float64) bool {
	if position < MinResumePosition.Seconds() {
		return false
	}
	if duration > 0 && duration-position < MinRemainingForResume.Seconds() {
		return false
	}
	return true
}

// ShouldOffer reports whether a stored position is worth prompting about. It
// mirrors ShouldStore so a position can never be saved and then ignored.
func (e Entry) ShouldOffer() bool {
	return ShouldStore(e.Position, e.Duration)
}

// file is the on-disk shape. Wrapping the slice in an object leaves room to add
// fields later without breaking older files.
type file struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// Store holds the recent-files list.
type Store struct {
	path string

	mu      sync.RWMutex
	entries map[string]Entry
}

// NewStore creates a store backed by dir/history.json and loads what is there.
// A corrupt file yields an empty history plus an error, never a failure to
// start.
func NewStore(dir string) (*Store, error) {
	s := &Store{
		path:    filepath.Join(dir, FileName),
		entries: make(map[string]Entry),
	}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, fmt.Errorf("history: reading %s: %w", s.path, err)
	}

	var parsed file
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return s, fmt.Errorf("history: parsing %s: %w", s.path, err)
	}

	for _, e := range parsed.Entries {
		if e.Path == "" {
			continue
		}
		s.entries[key(e.Path)] = e
	}
	return s, nil
}

// DefaultStore creates a store in the user's configuration directory.
func DefaultStore() (*Store, error) {
	dir, err := appdir.Config()
	if err != nil {
		return &Store{entries: make(map[string]Entry)}, err
	}
	return NewStore(dir)
}

// key normalises a path for lookup.
//
// Windows paths are case-insensitive, so the same file opened as C:\Videos\a.mkv
// and c:\videos\a.mkv must resolve to one history entry rather than two.
func key(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return strings.ToLower(filepath.Clean(path))
}

// Path is the history file's location.
func (s *Store) Path() string { return s.path }

// Lookup returns the stored entry for a path.
func (s *Store) Lookup(path string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[key(path)]
	return e, ok
}

// Record stores or updates a file's position and persists the list.
//
// Positions that fail ShouldStore are not merely skipped: any previously stored
// position for that file is cleared, so finishing a video removes its resume
// point instead of leaving a stale one two minutes from the end.
func (s *Store) Record(path, title string, position, duration float64) error {
	if path == "" {
		return nil
	}

	s.mu.Lock()
	k := key(path)
	entry, exists := s.entries[k]

	entry.Path = path
	entry.Filename = filepath.Base(path)
	if title != "" {
		entry.Title = title
	}
	if duration > 0 {
		entry.Duration = duration
	}
	entry.UpdatedAt = time.Now()

	if ShouldStore(position, duration) {
		entry.Position = position
	} else if position > 0 || !exists {
		// Watched to the end, or restarted from the beginning: forget where we
		// were, but keep the file in the recent list.
		entry.Position = 0
	}

	s.entries[k] = entry
	s.pruneLocked()
	snapshot := s.snapshotLocked()
	s.mu.Unlock()

	return s.save(snapshot)
}

// Forget removes one file from the history.
func (s *Store) Forget(path string) error {
	s.mu.Lock()
	delete(s.entries, key(path))
	snapshot := s.snapshotLocked()
	s.mu.Unlock()

	return s.save(snapshot)
}

// Clear empties the history.
func (s *Store) Clear() error {
	s.mu.Lock()
	s.entries = make(map[string]Entry)
	s.mu.Unlock()

	return s.save(nil)
}

// Recent returns up to limit entries, newest first, with Exists filled in.
//
// Entries whose file has gone are kept and flagged rather than deleted: a file
// on a disconnected drive is not the same thing as a file the user finished
// with, and silently losing the list would be worse than showing it greyed out.
func (s *Store) Recent(limit int) []Entry {
	s.mu.RLock()
	out := s.snapshotLocked()
	s.mu.RUnlock()

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}

	for i := range out {
		_, err := os.Stat(out[i].Path)
		out[i].Exists = err == nil
	}
	return out
}

// snapshotLocked returns the entries newest-first. Callers must hold the lock.
func (s *Store) snapshotLocked() []Entry {
	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].Path < out[j].Path // stable for same-instant writes
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

// pruneLocked drops the oldest entries beyond MaxEntries.
func (s *Store) pruneLocked() {
	if len(s.entries) <= MaxEntries {
		return
	}

	ordered := s.snapshotLocked()
	for _, e := range ordered[MaxEntries:] {
		delete(s.entries, key(e.Path))
	}
}

func (s *Store) save(entries []Entry) error {
	if s.path == "" {
		return nil
	}
	if entries == nil {
		entries = []Entry{}
	}

	// Exists is derived state; storing it would go stale immediately.
	for i := range entries {
		entries[i].Exists = false
	}

	data, err := json.MarshalIndent(file{Version: 1, Entries: entries}, "", "  ")
	if err != nil {
		return fmt.Errorf("history: encoding: %w", err)
	}
	data = append(data, '\n')

	if err := appdir.WriteFileAtomic(s.path, data, 0o644); err != nil {
		return fmt.Errorf("history: saving: %w", err)
	}
	return nil
}
