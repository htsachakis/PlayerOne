// Package settings persists the user's preferences as JSON.
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"playerone/internal/appdir"
)

// FileName is the settings file's name inside the config directory.
const FileName = "settings.json"

// Settings holds every preference that survives a restart.
//
// The JSON tags are also the wire format for the frontend, which reads and
// writes these directly, so the names are chosen to be meaningful in TypeScript.
type Settings struct {
	Volume        float64 `json:"volume"`
	Muted         bool    `json:"muted"`
	PlaybackSpeed float64 `json:"playbackSpeed"`

	AutoResume       bool `json:"autoResume"`
	FollowTranscript bool `json:"followTranscript"`

	SidePanelVisible bool `json:"sidePanelVisible"`
	SidePanelWidth   int  `json:"sidePanelWidth"`

	// Remembering the last-used languages lets a series of tutorials from the
	// same source open with the right tracks already selected.
	LastSubtitleLang string `json:"lastSubtitleLang"`
	LastAudioLang    string `json:"lastAudioLang"`

	// Track synchronisation offsets, in seconds, as mpv defines them.
	SubtitleDelay float64 `json:"subtitleDelay"`
	AudioDelay    float64 `json:"audioDelay"`

	// SubtitleScale multiplies the subtitle font size; 1 is mpv's own size.
	// Remembered because a viewer's preferred size follows from their screen and
	// eyesight, not from the file.
	SubtitleScale float64 `json:"subtitleScale"`

	// CheckForUpdates governs whether PlayerOne contacts GitHub on startup. It
	// is the only network access the application makes, so it is a setting
	// rather than a hidden behaviour.
	CheckForUpdates bool `json:"checkForUpdates"`
	// LastUpdateCheck is a Unix timestamp, kept so the check happens at most
	// once a day rather than on every launch.
	LastUpdateCheck int64 `json:"lastUpdateCheck"`
	// SkippedVersion is a release the user chose not to be reminded about.
	SkippedVersion string `json:"skippedVersion"`

	Window WindowState `json:"window"`

	// LogLevel is DEBUG, INFO, WARN or ERROR.
	LogLevel string `json:"logLevel"`
}

// WindowState remembers the main window's geometry between sessions.
type WindowState struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Maximised bool `json:"maximised"`
	// Valid distinguishes "never saved" from "saved as zero", so a first run
	// gets the default centred window rather than a window at (0,0).
	Valid bool `json:"valid"`
}

// Defaults returns the settings a fresh installation starts with.
func Defaults() Settings {
	return Settings{
		Volume:           100,
		Muted:            false,
		PlaybackSpeed:    1.0,
		AutoResume:       false,
		FollowTranscript: true,
		SidePanelVisible: true,
		SidePanelWidth:   380,
		SubtitleDelay:    0,
		AudioDelay:       0,
		SubtitleScale:    1.0,
		CheckForUpdates:  true,
		LogLevel:         "INFO",
		Window:           WindowState{Width: 1440, Height: 900},
	}
}

// Normalise clamps values into ranges the player can actually honour.
//
// This runs on every load because the settings file is user-editable and a
// hand-edited volume of 5000 or speed of 0 must not reach mpv.
func (s *Settings) Normalise() {
	def := Defaults()

	if !(s.Volume >= 0) || s.Volume > 150 {
		s.Volume = def.Volume
	}
	if !(s.PlaybackSpeed >= MinSpeed) || s.PlaybackSpeed > MaxSpeed {
		s.PlaybackSpeed = def.PlaybackSpeed
	}
	if s.SidePanelWidth < MinPanelWidth || s.SidePanelWidth > MaxPanelWidth {
		s.SidePanelWidth = def.SidePanelWidth
	}
	// mpv accepts large delays, but values beyond a minute are certainly a
	// mistake rather than a preference.
	if !(s.SubtitleDelay >= -60) || s.SubtitleDelay > 60 {
		s.SubtitleDelay = 0
	}
	if !(s.AudioDelay >= -60) || s.AudioDelay > 60 {
		s.AudioDelay = 0
	}
	if !(s.SubtitleScale >= MinSubtitleScale) || s.SubtitleScale > MaxSubtitleScale {
		s.SubtitleScale = def.SubtitleScale
	}
	if s.LogLevel == "" {
		s.LogLevel = def.LogLevel
	}
	if s.Window.Valid && (s.Window.Width < 640 || s.Window.Height < 480) {
		s.Window = def.Window
	}
	if s.Window.Width <= 0 || s.Window.Height <= 0 {
		s.Window.Width, s.Window.Height = def.Window.Width, def.Window.Height
	}
}

// Bounds the UI and mpv both respect.
const (
	MinSpeed      = 0.1
	MaxSpeed      = 16.0
	MinPanelWidth = 260
	MaxPanelWidth = 900

	MinSubtitleScale = 0.25
	MaxSubtitleScale = 4.0
)

// Store loads and saves settings, serialising concurrent access.
type Store struct {
	path string

	mu      sync.RWMutex
	current Settings
}

// NewStore creates a store backed by dir/settings.json and loads what is there.
//
// A missing or unreadable file is not an error: the defaults are used, because
// refusing to start over an unparseable preferences file would be absurd. The
// returned error describes what went wrong so the caller can log it.
func NewStore(dir string) (*Store, error) {
	s := &Store{
		path:    filepath.Join(dir, FileName),
		current: Defaults(),
	}

	loaded, err := load(s.path)
	if err != nil {
		return s, err
	}
	s.current = loaded
	return s, nil
}

// DefaultStore creates a store in the user's configuration directory.
func DefaultStore() (*Store, error) {
	dir, err := appdir.Config()
	if err != nil {
		return &Store{current: Defaults()}, err
	}
	return NewStore(dir)
}

func load(path string) (Settings, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Defaults(), nil // a first run, not a failure
		}
		return Defaults(), fmt.Errorf("settings: reading %s: %w", path, err)
	}

	// Start from the defaults so a file written by an older version, missing
	// newer fields, does not silently zero them.
	out := Defaults()
	if err := json.Unmarshal(raw, &out); err != nil {
		return Defaults(), fmt.Errorf("settings: parsing %s: %w", path, err)
	}

	out.Normalise()
	return out, nil
}

// Path is the settings file's location, shown in the Info tab.
func (s *Store) Path() string { return s.path }

// Get returns a copy of the current settings.
func (s *Store) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// Set replaces the settings in memory and writes them to disk.
func (s *Store) Set(next Settings) error {
	next.Normalise()

	s.mu.Lock()
	s.current = next
	s.mu.Unlock()

	return s.save(next)
}

// Update applies a mutation and persists the result. It is the safe way to
// change one field without racing another writer.
func (s *Store) Update(mutate func(*Settings)) (Settings, error) {
	s.mu.Lock()
	next := s.current
	mutate(&next)
	next.Normalise()
	s.current = next
	s.mu.Unlock()

	return next, s.save(next)
}

func (s *Store) save(v Settings) error {
	if s.path == "" {
		return nil // no config directory; run with in-memory settings
	}

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("settings: encoding: %w", err)
	}
	data = append(data, '\n')

	if err := appdir.WriteFileAtomic(s.path, data, 0o644); err != nil {
		return fmt.Errorf("settings: saving: %w", err)
	}
	return nil
}
