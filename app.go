package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"playerone/internal/appdir"
	"playerone/internal/branding"
	"playerone/internal/history"
	"playerone/internal/logging"
	"playerone/internal/media"
	"playerone/internal/notes"
	"playerone/internal/player"
	"playerone/internal/playlist"
	"playerone/internal/settings"
	"playerone/internal/tools"
	"playerone/internal/updater"
	"playerone/internal/version"
	"playerone/internal/winvideo"
)

// Frontend event names. Everything the backend pushes goes through one of these.
const (
	eventState        = "playback:state"
	eventTracks       = "player:tracks"
	eventMediaOpen    = "media:opened"
	eventMediaEnded   = "media:ended"
	eventError        = "app:error"
	eventReady        = "app:ready"
	eventResume       = "media:resume"
	eventNotesChanged = "notes:changed"
	// eventPointerMoved wakes the auto-hidden fullscreen controls. The page
	// cannot see the pointer while it is over the video, so this is the only
	// signal it gets.
	eventPointerMoved = "ui:pointer-moved"
)

// resumeSaveInterval is how often a playback position is written while playing.
//
// Ten seconds bounds what a crash can lose to ten seconds of progress, while
// keeping the write rate low enough to be irrelevant on any disk.
const resumeSaveInterval = 10 * time.Second

// App is the Wails-bound application. It is the only type that knows about both
// the user interface and the player.
type App struct {
	ctx context.Context
	log *logging.Logger

	settings *settings.Store
	history  *history.Store
	playlist *playlist.List
	notes    *notes.Store
	resolver *tools.Resolver
	updates  *updater.Client

	// Tool paths, resolved once at startup. Empty means unavailable.
	mpvPath     string
	ffmpegPath  string
	ffprobePath string

	videoHost *winvideo.Host

	mu     sync.RWMutex
	engine *player.MPVPlayer
	// startupError is shown by the frontend when the engine could not start.
	startupError string

	// currentPath is the file that is open, guarded by mu.
	currentPath string
	// pendingResume is a position to seek to once the file finishes loading.
	pendingResume float64
	// pendingSubtitle is an external subtitle to attach once the file is open,
	// set when a video and its subtitles are dropped together.
	pendingSubtitle string

	// mediaInfo is the assembled Info tab data for the current file.
	mediaInfo *player.MediaInfo

	// transcriptMu guards the cached transcript and the in-flight extraction.
	transcriptMu     sync.Mutex
	transcriptCancel context.CancelFunc
	transcriptCache  map[int]*TranscriptResult

	// fullscreen mirrors the window state so the video rectangle can be sized
	// correctly when controls auto-hide.
	fullscreen bool

	// The interface measures its video slot before the engine has finished
	// starting, so the last request is kept here and applied once the window
	// exists. Without this the video window stays at its initial size and the
	// user gets sound with no picture.
	videoBounds    winvideo.Bounds
	hasVideoBounds bool
	videoVisible   bool

	// pendingUpdate is the release a check found, kept so the install step acts
	// on something PlayerOne fetched rather than on anything the interface
	// hands back to it.
	pendingUpdate *updater.Release

	// pointerStop ends the fullscreen pointer watch; see watchPointer.
	pointerMu   sync.Mutex
	pointerStop chan struct{}

	initOnce sync.Once
	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewApp builds the application. Nothing that can fail happens here; startup
// work belongs in startup so failures can be reported to a running window.
func NewApp() *App {
	return &App{
		stopCh:          make(chan struct{}),
		notes:           notes.NewStore(),
		transcriptCache: make(map[int]*TranscriptResult),
	}
}

// startup runs when Wails has created the application context.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	configDir, dirErr := appdir.Config()

	// PLAYERONE_LOG overrides the stored preference, so a problem can be traced
	// without first having to reach the settings panel in an application that
	// may not be working.
	envLevel := strings.TrimSpace(os.Getenv("PLAYERONE_LOG"))

	level := logging.LevelInfo
	if envLevel != "" {
		level = logging.ParseLevel(envLevel)
	}

	if dirErr == nil {
		a.log = logging.NewWithFile(level, configDir, strings.ToLower(branding.Name))
	} else {
		a.log = logging.New(level)
		a.log.Error("app: no configuration directory available: %v", dirErr)
	}

	a.log.Info("app: %s %s starting", branding.Name, version.Detail())

	var err error
	if a.settings, err = settings.DefaultStore(); err != nil {
		a.log.Warn("app: settings could not be loaded, using defaults: %v", err)
	}
	if a.history, err = history.DefaultStore(); err != nil {
		a.log.Warn("app: history could not be loaded, starting empty: %v", err)
	}
	if a.playlist, err = playlist.Default(); err != nil {
		a.log.Warn("app: playlist could not be loaded, starting empty: %v", err)
	}

	// The stored preference applies only when the environment has not already
	// asked for a specific level.
	if envLevel == "" {
		a.log.SetLevel(logging.ParseLevel(a.settings.Get().LogLevel))
	}

	a.updates = updater.New()

	a.resolveTools()
}

// resolveTools locates mpv, ffmpeg and ffprobe and records what is missing.
func (a *App) resolveTools() {
	a.resolver = tools.NewResolver()
	a.log.Debug("app: searching for tools in %v then PATH", a.resolver.SearchDirs())

	var err error
	if a.mpvPath, err = a.resolver.Look(tools.MPV); err != nil {
		a.log.Error("app: %v", err)
	} else {
		a.log.Info("app: using mpv at %s", a.mpvPath)
	}

	// ffmpeg and ffprobe are optional. Their absence degrades features rather
	// than preventing playback, so it is logged as a warning and surfaced in
	// the Info tab rather than blocking startup.
	if a.ffmpegPath, err = a.resolver.Look(tools.FFmpeg); err != nil {
		a.log.Warn("app: %v", err)
	} else {
		a.log.Info("app: using ffmpeg at %s", a.ffmpegPath)
	}
	if a.ffprobePath, err = a.resolver.Look(tools.FFprobe); err != nil {
		a.log.Warn("app: %v", err)
	} else {
		a.log.Info("app: using ffprobe at %s", a.ffprobePath)
	}
}

// domReady runs once the page is loaded. The video window is created here
// because it needs the main window to exist, which is not guaranteed during
// startup.
func (a *App) domReady(context.Context) {
	a.initOnce.Do(func() {
		go a.initialiseEngine()
	})
}

// initialiseEngine creates the video window and starts mpv.
//
// It runs off the UI thread and reports its outcome through an event, so a slow
// or failing engine start never blocks the interface from appearing.
func (a *App) initialiseEngine() {
	if a.mpvPath == "" {
		a.failStartup(a.mpvMissingMessage())
		return
	}

	parent, err := winvideo.FindMainWindow(10 * time.Second)
	if err != nil {
		a.log.Error("app: %v", err)
		a.failStartup("PlayerOne could not attach its video surface to the application window. Restarting the application usually resolves this.")
		return
	}

	host, err := winvideo.NewHost(parent)
	if err != nil {
		a.log.Error("app: creating the video window: %v", err)
		a.failStartup("PlayerOne could not create the video surface. Restarting the application usually resolves this.")
		return
	}
	a.mu.Lock()
	a.videoHost = host
	a.mu.Unlock()

	current := a.settings.Get()

	engine, err := player.New(a.ctx, player.Config{
		MPVPath:       a.mpvPath,
		WindowID:      host.HWND(),
		InitialVolume: current.Volume,
		InitialSpeed:  current.PlaybackSpeed,
		InitialMuted:  current.Muted,
		Logger:        a.log,
		OnState:       a.onState,
		OnTracks:      a.onTracks,
		OnFileLoaded:  a.onFileLoaded,
		OnEndFile:     a.onEndFile,
		OnError:       a.onEngineError,
	})
	if err != nil {
		a.log.Error("app: starting the player: %v", err)
		host.Close()
		a.failStartup(fmt.Sprintf(
			"PlayerOne could not start the media engine.\n\n%v\n\nCheck that mpv.exe at %s is a working 64-bit Windows build.",
			err, a.mpvPath))
		return
	}

	a.mu.Lock()
	a.engine = engine
	a.mu.Unlock()

	// The interface has almost certainly already reported where the video goes.
	a.applyPendingVideoLayout()

	a.wg.Add(4)
	go a.runResumeSaver()
	go a.runStartupUpdateCheck()
	go a.runUpdateDownloadCleanup()
	go a.runNotesWatcher()

	a.log.Info("app: engine ready")
	a.emit(eventReady, a.Diagnostics())
}

func (a *App) failStartup(message string) {
	a.mu.Lock()
	a.startupError = message
	a.mu.Unlock()

	a.log.Error("app: startup failed: %s", strings.ReplaceAll(message, "\n", " "))
	a.emit(eventReady, a.Diagnostics())
}

func (a *App) mpvMissingMessage() string {
	var b strings.Builder
	b.WriteString("mpv was not found, so no video can be played.\n\n")
	b.WriteString("PlayerOne uses mpv as its media engine. Put mpv.exe in the bin folder next to ")
	b.WriteString(branding.Name)
	b.WriteString(".exe, or install mpv so that it is on your PATH.\n\nSearched:")
	for _, dir := range a.resolver.SearchDirs() {
		b.WriteString("\n  ")
		b.WriteString(dir)
	}
	b.WriteString("\n  directories on PATH")
	return b.String()
}

// shutdown persists state and stops everything. Wails calls it before the
// window closes.
func (a *App) shutdown(context.Context) {
	a.stopOnce.Do(func() {
		a.log.Info("app: shutting down")
		close(a.stopCh)

		a.saveResumePosition()
		a.cancelTranscript()
		a.stopPointerWatch()
		a.wg.Wait()

		if engine := a.currentEngine(); engine != nil {
			if err := engine.Close(); err != nil {
				a.log.Warn("app: closing the player: %v", err)
			}
		}
		if host := a.host(); host != nil {
			host.Close()
		}
		if a.log != nil {
			_ = a.log.Close()
		}
	})
}

// host returns the video window, or nil before the engine has started.
func (a *App) host() *winvideo.Host {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.videoHost
}

// currentEngine returns the player, or nil if the engine never started.
func (a *App) currentEngine() *player.MPVPlayer {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.engine
}

// requireEngine returns the player or a message suitable for showing a user.
func (a *App) requireEngine() (*player.MPVPlayer, error) {
	if engine := a.currentEngine(); engine != nil {
		return engine, nil
	}

	a.mu.RLock()
	msg := a.startupError
	a.mu.RUnlock()

	if msg == "" {
		msg = "The media engine is still starting. Try again in a moment."
	}
	return nil, fmt.Errorf("%s", msg)
}

func (a *App) emit(name string, data ...any) {
	if a.ctx == nil {
		return
	}
	wailsruntime.EventsEmit(a.ctx, name, data...)
}

// --- Player callbacks. All run on the player's single pump goroutine. ---

func (a *App) onState(state player.PlaybackState) {
	a.emit(eventState, state)
}

func (a *App) onTracks(tracks []player.Track, chapters []player.Chapter) {
	a.emit(eventTracks, map[string]any{
		"tracks":   tracks,
		"chapters": chapters,
	})
}

// onFileLoaded completes the open sequence once mpv reports the file is ready.
func (a *App) onFileLoaded(path string) {
	a.log.Info("app: loaded %s", path)

	a.mu.Lock()
	resume := a.pendingResume
	a.pendingResume = 0
	subtitle := a.pendingSubtitle
	a.pendingSubtitle = ""
	a.mu.Unlock()

	// A subtitle dropped alongside the video can only attach now that mpv has
	// the file open.
	if subtitle != "" {
		if err := a.AddSubtitle(subtitle); err != nil {
			a.log.Warn("app: could not attach the dropped subtitle: %v", err)
			a.emit(eventError, err.Error())
		}
	}

	// Applying the remembered track languages and the resume position here, and
	// not at load time, is deliberate: neither the track list nor a seekable
	// timeline exists until the file is open.
	a.applyPreferredTracks()

	if resume > 0 {
		if engine := a.currentEngine(); engine != nil {
			if err := engine.Seek(a.ctx, resume); err != nil {
				a.log.Warn("app: could not resume at %.1fs: %v", resume, err)
			} else {
				a.log.Info("app: resumed at %.1fs", resume)
			}
		}
	}

	a.refreshMediaInfo()
	a.invalidateTranscript()

	// A file mpv opened by itself - a drop on the video surface, which the
	// WebView never sees - has been through none of Open's bookkeeping. Bring it
	// back in line so history, resume and the queue behave identically however
	// the file arrived.
	a.reconcileExternalOpen()

	// The queue always reflects what is playing, even for a file opened outside
	// it, so "next" is meaningful no matter how the file was opened.
	if a.playlist != nil {
		if engine := a.currentEngine(); engine != nil {
			state := engine.State()
			a.playlist.SelectPath(state.Path)
			a.playlist.UpdateCurrent(state.Title, state.Duration)
		}
		a.emitPlaylist()
	}

	if host := a.host(); host != nil {
		host.Show()
	}

	a.emit(eventMediaOpen, a.MediaInfo())
}

func (a *App) onEndFile(reason string) {
	a.log.Debug("app: playback stopped (%s)", reason)

	// Only a natural end advances the queue. mpv also reports end-file when a
	// file is replaced, and advancing on that would load the next file, which
	// stops the current one, which advances again, for ever.
	if reason != "eof" {
		return
	}

	a.saveResumePosition()
	a.emit(eventMediaEnded)

	if a.playlist != nil && a.playlist.Len() > 0 {
		if err := a.advance(true); err != nil {
			a.log.Warn("app: could not advance the playlist: %v", err)
			a.emit(eventError, err.Error())
		}
	}
}

func (a *App) onEngineError(message string) {
	a.log.Error("app: engine error: %s", message)
	a.emit(eventError, message)
}

// reconcileExternalOpen handles a file that started playing without Open being
// called, which happens when one is dropped onto the video surface.
//
// mpv owns that drop, because the video is a native window the WebView cannot
// see. Rather than leaving such a file outside the application's model, it is
// recorded in history here and its resume position offered, exactly as an
// Open would have done.
func (a *App) reconcileExternalOpen() {
	engine := a.currentEngine()
	if engine == nil {
		return
	}

	state := engine.State()
	if state.Path == "" {
		return
	}

	a.mu.Lock()
	known := a.currentPath
	if samePath(known, state.Path) {
		a.mu.Unlock()
		return // opened through Open; already accounted for
	}
	a.currentPath = state.Path
	a.mu.Unlock()

	a.log.Info("app: %s was opened by mpv (dropped on the video)", filepath.Base(state.Path))

	if a.history == nil {
		return
	}
	if err := a.history.Record(state.Path, state.Title, 0, state.Duration); err != nil {
		a.log.Warn("app: could not record the dropped file: %v", err)
	}

	entry, ok := a.history.Lookup(state.Path)
	if !ok || !entry.ShouldOffer() {
		return
	}

	if a.settings.Get().AutoResume {
		if err := engine.Seek(a.ctx, entry.Position); err != nil {
			a.log.Warn("app: could not resume the dropped file: %v", err)
		}
		return
	}

	// The interface owns all prompting, so it is asked to offer the choice.
	if err := engine.Pause(a.ctx); err != nil {
		a.log.Debug("app: could not pause before offering to resume: %v", err)
	}
	a.emit(eventResume, map[string]any{
		"position": entry.Position,
		"filename": filepath.Base(state.Path),
	})
}

// samePath compares two paths the way Windows does.
func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// applyPreferredTracks selects the audio and subtitle languages the user last
// chose, when this file offers them.
func (a *App) applyPreferredTracks() {
	engine := a.currentEngine()
	if engine == nil {
		return
	}

	current := a.settings.Get()
	tracks := engine.Tracks()

	if lang := current.LastAudioLang; lang != "" {
		if id, ok := findTrackByLanguage(tracks, player.TrackAudio, lang); ok {
			if err := engine.SetAudioTrack(a.ctx, id); err != nil {
				a.log.Debug("app: could not select the remembered audio language %q: %v", lang, err)
			}
		}
	}

	switch current.LastSubtitleLang {
	case "":
		// No preference recorded; leave mpv's own default selection alone.
	case subtitlesOff:
		if err := engine.DisableSubtitles(a.ctx); err != nil {
			a.log.Debug("app: could not turn subtitles off: %v", err)
		}
	default:
		if id, ok := findTrackByLanguage(tracks, player.TrackSubtitle, current.LastSubtitleLang); ok {
			if err := engine.SetSubtitleTrack(a.ctx, id); err != nil {
				a.log.Debug("app: could not select the remembered subtitle language %q: %v", current.LastSubtitleLang, err)
			}
		}
	}

	if current.SubtitleDelay != 0 {
		_ = engine.SetSubtitleDelay(a.ctx, current.SubtitleDelay)
	}
	if current.AudioDelay != 0 {
		_ = engine.SetAudioDelay(a.ctx, current.AudioDelay)
	}
	if current.SubtitleScale != 1 && current.SubtitleScale > 0 {
		_ = engine.SetSubtitleScale(a.ctx, current.SubtitleScale)
	}
}

// subtitlesOff is the sentinel stored in settings when the user turned subtitles
// off, as distinct from never having chosen.
const subtitlesOff = "\x00off"

func findTrackByLanguage(tracks []player.Track, kind, language string) (int, bool) {
	// Compare by display name so "gre", "ell" and "el" all match a remembered
	// preference of "Greek".
	want := player.LanguageName(language)
	if want == "" {
		return 0, false
	}

	for _, t := range tracks {
		if t.Type != kind {
			continue
		}
		if strings.EqualFold(player.LanguageName(t.Language), want) {
			return t.ID, true
		}
	}
	return 0, false
}

// refreshMediaInfo assembles the Info tab data for the open file.
func (a *App) refreshMediaInfo() {
	engine := a.currentEngine()
	if engine == nil {
		return
	}

	state := engine.State()
	path := state.Path
	if path == "" {
		return
	}

	// ffprobe is bounded: a damaged file can make it work hard, and the Info tab
	// is not worth stalling the open sequence for.
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	var (
		probe    *media.ProbeResult
		probeErr error
	)
	if a.ffprobePath != "" {
		probe, probeErr = media.Probe(ctx, a.ffprobePath, path)
		if probeErr != nil {
			a.log.Warn("app: %v", probeErr)
		}
	} else {
		probeErr = fmt.Errorf("ffprobe was not found, so some details are unavailable")
	}

	info := media.BuildInfo(path, state, engine.Tracks(), engine.Chapters(), probe, probeErr)

	a.mu.Lock()
	a.mediaInfo = &info
	a.mu.Unlock()
}

// runResumeSaver periodically persists the playback position.
func (a *App) runResumeSaver() {
	defer a.wg.Done()

	ticker := time.NewTicker(resumeSaveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.saveResumePosition()
		}
	}
}

// notesPollInterval is how often the bound notes file is checked for changes
// made outside PlayerOne.
//
// The file is plain text, so it will be edited in Notepad and rewritten by
// OneDrive and Dropbox. Polling rather than watching the filesystem is the
// reliable choice across network shares and sync folders, where change
// notifications are unreliable or absent, and two seconds is far below the
// threshold at which a person notices a delay.
const notesPollInterval = 2 * time.Second

// runNotesWatcher re-reads the notes file when it changes on disk.
func (a *App) runNotesWatcher() {
	defer a.wg.Done()

	ticker := time.NewTicker(notesPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			if a.notes == nil {
				continue
			}
			changed, err := a.notes.Reload()
			if err != nil {
				a.log.Warn("app: %v", err)
				continue
			}
			if changed {
				a.emitNotes()
			}
		}
	}
}

// saveResumePosition records where playback has reached.
//
// history.Record decides whether the position is worth keeping, so this can be
// called freely without duplicating the first-ten-seconds and nearly-finished
// rules.
func (a *App) saveResumePosition() {
	engine := a.currentEngine()
	if engine == nil || a.history == nil {
		return
	}

	state := engine.State()
	if state.Path == "" || !state.FileLoaded {
		return
	}

	if err := a.history.Record(state.Path, state.Title, state.Position, state.Duration); err != nil {
		a.log.Warn("app: could not save the playback position: %v", err)
	}
}

// looksLikeMediaFile reports whether a path is something worth trying to open.
//
// The list is intentionally permissive: mpv plays far more than this, and the
// check only exists so that dropping a .zip produces a clear message instead of
// an mpv failure.
// This list is mirrored by PLAYERONE_EACH_EXT in
// build/windows/installer/project.nsi, which registers the same extensions with
// Windows. Add to one and add to the other, or the installer will offer to open
// something the application then refuses.
var mediaExtensions = map[string]bool{
	// Video
	".mkv": true, ".mp4": true, ".webm": true, ".mov": true, ".avi": true,
	".m4v": true, ".mpg": true, ".mpeg": true, ".m2v": true, ".ts": true,
	".m2ts": true, ".mts": true, ".wmv": true, ".asf": true, ".flv": true,
	".f4v": true, ".ogv": true, ".3gp": true, ".3g2": true, ".vob": true,
	".divx": true, ".rmvb": true, ".mxf": true,

	// Audio
	".mp3": true, ".m4a": true, ".m4b": true, ".flac": true, ".opus": true,
	".wav": true, ".aac": true, ".ogg": true, ".oga": true, ".wma": true,
	".mka": true, ".ape": true, ".alac": true, ".aiff": true, ".dsf": true,
}

func looksLikeMediaFile(path string) bool {
	return mediaExtensions[strings.ToLower(filepath.Ext(path))]
}

// CurrentPath is the file that is open, empty when nothing is.
func (a *App) CurrentPath() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentPath
}
