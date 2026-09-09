package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDefaultsMatchTheSpecifiedValues(t *testing.T) {
	d := Defaults()

	if d.Volume != 100 {
		t.Errorf("Volume = %v, want 100", d.Volume)
	}
	if d.PlaybackSpeed != 1.0 {
		t.Errorf("PlaybackSpeed = %v, want 1.0", d.PlaybackSpeed)
	}
	if d.AutoResume {
		t.Error("AutoResume = true, want false")
	}
	if !d.FollowTranscript {
		t.Error("FollowTranscript = false, want true")
	}
	if !d.SidePanelVisible {
		t.Error("SidePanelVisible = false, want true")
	}
	if d.SidePanelWidth < 320 || d.SidePanelWidth > 420 {
		t.Errorf("SidePanelWidth = %d, want the documented 320-420 range", d.SidePanelWidth)
	}
}

func TestNewStoreOnEmptyDirectoryUsesDefaults(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if got := store.Get(); got != Defaults() {
		t.Errorf("got %+v, want the defaults", got)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	dir := t.TempDir()

	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	want := Defaults()
	want.Volume = 62
	want.Muted = true
	want.PlaybackSpeed = 1.75
	want.AutoResume = true
	want.FollowTranscript = false
	want.SidePanelVisible = false
	want.SidePanelWidth = 420
	want.LastSubtitleLang = "eng"
	want.LastAudioLang = "gre"
	want.SubtitleDelay = -0.5
	want.AudioDelay = 0.25
	want.Window = WindowState{Width: 1600, Height: 1000, X: 40, Y: 20, Maximised: true, Valid: true}

	if err := store.Set(want); err != nil {
		t.Fatalf("Set: %v", err)
	}

	reloaded, err := NewStore(dir)
	if err != nil {
		t.Fatalf("reloading: %v", err)
	}
	if got := reloaded.Get(); got != want {
		t.Errorf("round trip lost data:\n got %+v\nwant %+v", got, want)
	}
}

func TestUpdateMutatesAndPersists(t *testing.T) {
	dir := t.TempDir()

	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	got, err := store.Update(func(s *Settings) { s.Volume = 33 })
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Volume != 33 {
		t.Errorf("returned Volume = %v, want 33", got.Volume)
	}

	reloaded, err := NewStore(dir)
	if err != nil {
		t.Fatalf("reloading: %v", err)
	}
	if reloaded.Get().Volume != 33 {
		t.Errorf("persisted Volume = %v, want 33", reloaded.Get().Volume)
	}
}

// A settings file from an older release will not contain newer fields. Those
// must fall back to their defaults rather than to Go zero values.
func TestPartialFileKeepsDefaultsForMissingFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	if err := os.WriteFile(path, []byte(`{"volume": 55}`), 0o644); err != nil {
		t.Fatalf("writing partial settings: %v", err)
	}

	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	got := store.Get()
	if got.Volume != 55 {
		t.Errorf("Volume = %v, want the value from the file (55)", got.Volume)
	}
	if got.PlaybackSpeed != 1.0 {
		t.Errorf("PlaybackSpeed = %v, want the default 1.0, not the zero value", got.PlaybackSpeed)
	}
	if !got.FollowTranscript {
		t.Error("FollowTranscript = false, want the default true")
	}
	if got.SidePanelWidth != Defaults().SidePanelWidth {
		t.Errorf("SidePanelWidth = %d, want the default", got.SidePanelWidth)
	}
}

// A corrupt file must report the problem but still yield a usable store.
func TestCorruptFileFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{ this is not json"), 0o644); err != nil {
		t.Fatalf("writing corrupt settings: %v", err)
	}

	store, err := NewStore(dir)
	if err == nil {
		t.Error("expected an error describing the corrupt file")
	}
	if store == nil {
		t.Fatal("NewStore returned no store for a corrupt file")
	}
	if got := store.Get(); got != Defaults() {
		t.Errorf("got %+v, want the defaults", got)
	}
}

func TestNormaliseClampsOutOfRangeValues(t *testing.T) {
	cases := []struct {
		name  string
		in    Settings
		check func(*testing.T, Settings)
	}{
		{
			name: "absurd volume",
			in:   Settings{Volume: 5000},
			check: func(t *testing.T, s Settings) {
				if s.Volume != 100 {
					t.Errorf("Volume = %v, want 100", s.Volume)
				}
			},
		},
		{
			name: "negative volume",
			in:   Settings{Volume: -20},
			check: func(t *testing.T, s Settings) {
				if s.Volume != 100 {
					t.Errorf("Volume = %v, want 100", s.Volume)
				}
			},
		},
		{
			name: "zero speed would freeze playback",
			in:   Settings{PlaybackSpeed: 0},
			check: func(t *testing.T, s Settings) {
				if s.PlaybackSpeed != 1.0 {
					t.Errorf("PlaybackSpeed = %v, want 1.0", s.PlaybackSpeed)
				}
			},
		},
		{
			name: "absurd speed",
			in:   Settings{PlaybackSpeed: 500},
			check: func(t *testing.T, s Settings) {
				if s.PlaybackSpeed != 1.0 {
					t.Errorf("PlaybackSpeed = %v, want 1.0", s.PlaybackSpeed)
				}
			},
		},
		{
			name: "8x is a supported speed and must survive",
			in:   Settings{PlaybackSpeed: 8},
			check: func(t *testing.T, s Settings) {
				if s.PlaybackSpeed != 8 {
					t.Errorf("PlaybackSpeed = %v, want 8 to be kept", s.PlaybackSpeed)
				}
			},
		},
		{
			name: "panel too narrow",
			in:   Settings{SidePanelWidth: 10},
			check: func(t *testing.T, s Settings) {
				if s.SidePanelWidth != Defaults().SidePanelWidth {
					t.Errorf("SidePanelWidth = %d, want the default", s.SidePanelWidth)
				}
			},
		},
		{
			name: "panel wider than any screen",
			in:   Settings{SidePanelWidth: 100000},
			check: func(t *testing.T, s Settings) {
				if s.SidePanelWidth != Defaults().SidePanelWidth {
					t.Errorf("SidePanelWidth = %d, want the default", s.SidePanelWidth)
				}
			},
		},
		{
			name: "absurd subtitle delay",
			in:   Settings{SubtitleDelay: 9999},
			check: func(t *testing.T, s Settings) {
				if s.SubtitleDelay != 0 {
					t.Errorf("SubtitleDelay = %v, want 0", s.SubtitleDelay)
				}
			},
		},
		{
			name: "reasonable negative subtitle delay is kept",
			in:   Settings{SubtitleDelay: -2.5},
			check: func(t *testing.T, s Settings) {
				if s.SubtitleDelay != -2.5 {
					t.Errorf("SubtitleDelay = %v, want -2.5 to be kept", s.SubtitleDelay)
				}
			},
		},
		{
			name: "tiny saved window",
			in:   Settings{Window: WindowState{Width: 20, Height: 20, Valid: true}},
			check: func(t *testing.T, s Settings) {
				if s.Window.Width < 640 || s.Window.Height < 480 {
					t.Errorf("Window = %+v, want a usable size", s.Window)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.in
			s.Normalise()
			tc.check(t, s)
		})
	}
}

func TestNormaliseRejectsNaN(t *testing.T) {
	zero := 0.0
	nan := zero / zero

	s := Settings{Volume: nan, PlaybackSpeed: nan, SubtitleDelay: nan, AudioDelay: nan}
	s.Normalise()

	if s.Volume != 100 {
		t.Errorf("Volume = %v, want 100", s.Volume)
	}
	if s.PlaybackSpeed != 1.0 {
		t.Errorf("PlaybackSpeed = %v, want 1.0", s.PlaybackSpeed)
	}
	if s.SubtitleDelay != 0 || s.AudioDelay != 0 {
		t.Errorf("delays = %v/%v, want 0/0", s.SubtitleDelay, s.AudioDelay)
	}
}

// Out-of-range values reaching Set must be clamped before they are stored, not
// only when they are read back.
func TestSetNormalisesBeforePersisting(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	bad := Defaults()
	bad.PlaybackSpeed = 0
	if err := store.Set(bad); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if store.Get().PlaybackSpeed != 1.0 {
		t.Errorf("in-memory PlaybackSpeed = %v, want 1.0", store.Get().PlaybackSpeed)
	}

	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}
	var onDisk Settings
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("parsing settings: %v", err)
	}
	if onDisk.PlaybackSpeed != 1.0 {
		t.Errorf("persisted PlaybackSpeed = %v, want 1.0", onDisk.PlaybackSpeed)
	}
}

func TestWrittenFileIsReadableJSON(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := store.Update(func(s *Settings) { s.LastAudioLang = "eng" }); err != nil {
		t.Fatalf("Update: %v", err)
	}

	raw, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}

	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("settings file is not valid JSON: %v", err)
	}
	if _, ok := generic["lastAudioLang"]; !ok {
		t.Errorf("expected a lastAudioLang key in %s", raw)
	}
}

func TestConcurrentUpdatesAreSerialised(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = store.Update(func(s *Settings) { s.Volume = float64(n%100 + 1) })
			_ = store.Get()
		}(i)
	}
	wg.Wait()

	// The file must still parse: an interleaved write would corrupt it.
	reloaded, err := NewStore(dir)
	if err != nil {
		t.Fatalf("settings file was corrupted by concurrent writes: %v", err)
	}
	if v := reloaded.Get().Volume; v < 1 || v > 100 {
		t.Errorf("Volume = %v, want one of the written values", v)
	}
}
