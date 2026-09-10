// Package player drives media playback.
//
// The Player interface is the boundary that keeps mpv out of the rest of the
// application: no property name, IPC message shape or command-line flag appears
// outside this package.
package player

import (
	"context"

	"playerone/internal/logging"
)

// Player controls playback of a single media file at a time.
//
// Implementations are safe for concurrent use: the frontend issues commands from
// WebView callbacks while the event pump reads state.
type Player interface {
	// Load opens a file, replacing whatever was playing.
	Load(ctx context.Context, path string) error
	// Unload closes the current file and returns the player to idle.
	Unload(ctx context.Context) error

	Play(ctx context.Context) error
	Pause(ctx context.Context) error
	TogglePause(ctx context.Context) error

	// Seek moves to an absolute position in seconds.
	Seek(ctx context.Context, seconds float64) error
	// SeekRelative moves by an offset in seconds, positive or negative.
	SeekRelative(ctx context.Context, delta float64) error
	// SeekChapter jumps to a chapter by index.
	SeekChapter(ctx context.Context, index int) error

	SetVolume(ctx context.Context, value float64) error
	SetMute(ctx context.Context, muted bool) error
	SetSpeed(ctx context.Context, speed float64) error
	// SetReverse switches between forward and backward playback.
	SetReverse(ctx context.Context, reverse bool) error

	SetSubtitleTrack(ctx context.Context, id int) error
	DisableSubtitles(ctx context.Context) error
	SetAudioTrack(ctx context.Context, id int) error
	// AddSubtitleFile attaches an external subtitle file and selects it.
	AddSubtitleFile(ctx context.Context, path string) error

	SetSubtitleDelay(ctx context.Context, seconds float64) error
	SetAudioDelay(ctx context.Context, seconds float64) error
	// SetSubtitleScale resizes subtitles; 1 is mpv's own size.
	SetSubtitleScale(ctx context.Context, scale float64) error

	// State returns the current snapshot. Cheap enough to call freely.
	State() PlaybackState
	Chapters() []Chapter
	Tracks() []Track

	Close() error
}

// StateFunc receives a state snapshot at the event pump's rate.
type StateFunc func(PlaybackState)

// TracksFunc receives the track and chapter lists when they change, which is
// rare enough to deliver immediately rather than on the state ticker.
type TracksFunc func(tracks []Track, chapters []Chapter)

// ErrorFunc receives a message intended for the user, with the technical detail
// already logged separately.
type ErrorFunc func(message string)

// Config configures an MPVPlayer.
type Config struct {
	// MPVPath is the mpv executable, already resolved.
	MPVPath string

	// WindowID is the HWND mpv renders into. Zero makes mpv open its own
	// window, which is only useful when debugging outside the Wails shell.
	WindowID uintptr

	// InitialVolume and InitialSpeed apply from the first frame, so restored
	// settings never cause an audible jump.
	InitialVolume float64
	InitialSpeed  float64
	InitialMuted  bool

	Logger *logging.Logger

	// OnState is called at the event pump's rate with the latest snapshot.
	OnState StateFunc
	// OnTracks is called when the track or chapter lists change.
	OnTracks TracksFunc
	// OnFileLoaded is called once a file is open and its metadata is readable.
	OnFileLoaded func(path string)
	// OnEndFile is called when a file stops playing. The reason is mpv's own:
	// only "eof" means playback finished naturally, and the others ("stop",
	// "redirect", "quit", "error") happen when a file is replaced. Auto-advance
	// depends on the distinction, since advancing on "stop" would load the next
	// file, which stops the current one, which advances again, without end.
	OnEndFile func(reason string)
	// OnError reports a user-facing problem.
	OnError ErrorFunc
}
