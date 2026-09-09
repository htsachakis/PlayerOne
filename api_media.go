package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"playerone/internal/branding"
	"playerone/internal/media"
	"playerone/internal/player"
	"playerone/internal/tools"
)

// OpenResult tells the frontend what happened when a file was opened, including
// whether a resume prompt is warranted.
type OpenResult struct {
	Path     string `json:"path"`
	Filename string `json:"filename"`

	// ResumeAvailable means a saved position exists and is worth offering.
	ResumeAvailable bool    `json:"resumeAvailable"`
	ResumePosition  float64 `json:"resumePosition"`
	// AutoResumed means the position was applied without asking, because the
	// user turned "always resume" on.
	AutoResumed bool `json:"autoResumed"`
}

// Diagnostics describes the environment, for the Info tab and the startup
// screen.
type Diagnostics struct {
	AppName    string `json:"appName"`
	Tagline    string `json:"tagline"`
	EngineReady bool  `json:"engineReady"`
	// StartupError is a user-facing explanation of why the engine is unavailable.
	StartupError string `json:"startupError"`

	MPVPath     string `json:"mpvPath"`
	FFmpegPath  string `json:"ffmpegPath"`
	FFprobePath string `json:"ffprobePath"`

	SearchDirs []string `json:"searchDirs"`

	SettingsPath string `json:"settingsPath"`
	HistoryPath  string `json:"historyPath"`
	PlaylistPath string `json:"playlistPath"`

	// HWDec names the decoder mpv actually chose, which is the first thing to
	// check when playback stutters.
	HWDec string `json:"hwdec"`
}

// Diagnostics reports the current environment.
func (a *App) Diagnostics() Diagnostics {
	a.mu.RLock()
	startupErr := a.startupError
	a.mu.RUnlock()

	d := Diagnostics{
		AppName:      branding.Name,
		Tagline:      branding.Tagline,
		EngineReady:  a.currentEngine() != nil,
		StartupError: startupErr,
		MPVPath:      a.mpvPath,
		FFmpegPath:   a.ffmpegPath,
		FFprobePath:  a.ffprobePath,
	}

	if a.resolver != nil {
		d.SearchDirs = a.resolver.SearchDirs()
	}
	if a.settings != nil {
		d.SettingsPath = a.settings.Path()
	}
	if a.history != nil {
		d.HistoryPath = a.history.Path()
	}
	if a.playlist != nil {
		d.PlaylistPath = a.playlist.Path()
	}
	if engine := a.currentEngine(); engine != nil {
		d.HWDec = engine.State().HWDec
	}

	return d
}

// ChooseFile shows the system open dialog and returns the chosen path, or an
// empty string if the user cancelled.
func (a *App) ChooseFile() (string, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Open video",
		Filters: []wailsruntime.FileFilter{
			{
				DisplayName: "Video files",
				Pattern:     "*.mkv;*.mp4;*.webm;*.mov;*.avi;*.m4v;*.ts;*.m2ts;*.mpg;*.mpeg;*.wmv;*.flv;*.ogv",
			},
			{DisplayName: "Audio files", Pattern: "*.mp3;*.m4a;*.flac;*.opus;*.wav;*.aac;*.ogg"},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("could not show the file dialog: %w", err)
	}
	return path, nil
}

// Open loads a media file.
//
// The returned OpenResult carries the resume decision rather than the backend
// prompting: the interface owns all user interaction, and a native dialog over
// the video would collide with the native video window.
func (a *App) Open(path string) (OpenResult, error) {
	var result OpenResult

	engine, err := a.requireEngine()
	if err != nil {
		return result, err
	}

	path = strings.TrimSpace(path)
	if path == "" {
		return result, fmt.Errorf("no file was given")
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return result, fmt.Errorf("%s no longer exists. It may have been moved, renamed or deleted.", filepath.Base(abs))
		}
		return result, fmt.Errorf("%s could not be opened: %v", filepath.Base(abs), err)
	}
	if info.IsDir() {
		return result, fmt.Errorf("%s is a folder, not a video file.", filepath.Base(abs))
	}

	if media.IsSubtitleFile(abs) {
		return result, fmt.Errorf("%s is a subtitle file. Open a video first, then load this as a subtitle.", filepath.Base(abs))
	}

	// Save where the previous file had reached before replacing it.
	a.saveResumePosition()

	result.Path = abs
	result.Filename = filepath.Base(abs)

	if a.history != nil {
		if entry, ok := a.history.Lookup(abs); ok && entry.ShouldOffer() {
			if a.settings.Get().AutoResume {
				a.mu.Lock()
				a.pendingResume = entry.Position
				a.mu.Unlock()
				result.AutoResumed = true
			} else {
				result.ResumeAvailable = true
			}
			result.ResumePosition = entry.Position
		}
	}

	a.mu.Lock()
	a.currentPath = abs
	a.mediaInfo = nil
	a.mu.Unlock()

	a.invalidateTranscript()

	if err := engine.Load(a.ctx, abs); err != nil {
		return result, fmt.Errorf("%s could not be played: %v", filepath.Base(abs), err)
	}

	// Record the open immediately so the file reaches the recent list even if
	// the viewer closes the window seconds later.
	if a.history != nil {
		if err := a.history.Record(abs, "", 0, 0); err != nil {
			a.log.Warn("app: could not record the recent file: %v", err)
		}
	}

	return result, nil
}

// ChooseSubtitle shows a dialog for picking an external subtitle file.
func (a *App) ChooseSubtitle() (string, error) {
	patterns := make([]string, 0, 8)
	for _, ext := range media.SubtitleExtensions() {
		patterns = append(patterns, "*"+ext)
	}

	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Load subtitle file",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Subtitle files", Pattern: strings.Join(patterns, ";")},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("could not show the file dialog: %w", err)
	}
	return path, nil
}

// AddSubtitle attaches an external subtitle file to the current media and
// selects it.
func (a *App) AddSubtitle(path string) error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}

	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("no subtitle file was given")
	}
	if !engine.State().FileLoaded {
		return fmt.Errorf("open a video before loading a subtitle file")
	}

	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%s could not be read: %v", filepath.Base(path), err)
	}
	if !media.IsSubtitleFile(path) {
		return fmt.Errorf("%s is not a subtitle format PlayerOne recognises (%s)",
			filepath.Base(path), strings.Join(media.SubtitleExtensions(), ", "))
	}

	if err := engine.AddSubtitleFile(a.ctx, path); err != nil {
		return fmt.Errorf("%s could not be loaded: %v", filepath.Base(path), err)
	}

	a.invalidateTranscript()
	return nil
}

// HandleDrop routes dropped items.
//
// One video is opened. Several videos, or a folder, become a queue, because
// dropping ten lessons and having nine ignored is never what was meant. A
// subtitle attaches to whatever is already playing.
func (a *App) HandleDrop(paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	var videos []string
	var subtitle string
	var folders []string

	for _, p := range paths {
		switch {
		case looksLikeMediaFile(p):
			videos = append(videos, p)
		case subtitle == "" && media.IsSubtitleFile(p):
			subtitle = p
		default:
			if info, err := os.Stat(p); err == nil && info.IsDir() {
				folders = append(folders, p)
			}
		}
	}

	// Dropping several files, or a folder, builds a queue rather than playing
	// only the first one and discarding the rest.
	if len(videos) > 1 || len(folders) > 0 {
		_, err := a.AddToPlaylist(append(videos, folders...))
		return err
	}

	var video string
	if len(videos) == 1 {
		video = videos[0]
	}

	if video != "" {
		if _, err := a.Open(video); err != nil {
			return err
		}
		_ = a.Play()
		// A video and its subtitles dropped together: mpv needs the file open
		// before an external subtitle can attach to it, and the attach happens
		// once the load completes.
		if subtitle != "" {
			a.mu.Lock()
			a.pendingSubtitle = subtitle
			a.mu.Unlock()
		}
		return nil
	}

	if subtitle != "" {
		return a.AddSubtitle(subtitle)
	}

	names := make([]string, 0, len(paths))
	for _, p := range paths {
		names = append(names, filepath.Base(p))
	}
	return fmt.Errorf("PlayerOne cannot open %s. Drop a video file, or a subtitle file while a video is playing.",
		strings.Join(names, ", "))
}

// MediaInfo returns the Info tab's data for the open file, or nil when nothing
// is open.
func (a *App) MediaInfo() *player.MediaInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.mediaInfo
}

// ToolStatus reports which optional tools are missing and what that costs, so
// the Info tab can explain a reduced feature set rather than leaving it a
// mystery.
func (a *App) ToolStatus() []string {
	var notes []string

	if a.ffmpegPath == "" {
		notes = append(notes, (&tools.NotFoundError{Tool: tools.FFmpeg}).FriendlyMessage())
	}
	if a.ffprobePath == "" {
		notes = append(notes, (&tools.NotFoundError{Tool: tools.FFprobe}).FriendlyMessage())
	}

	if notes == nil {
		return []string{}
	}
	return notes
}
