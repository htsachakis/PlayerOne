package player

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"playerone/internal/logging"
	"playerone/internal/mpvipc"
)

// commandTimeout bounds every IPC round trip.
//
// The UI must never hang on mpv. Two seconds is far longer than a local named
// pipe ever needs and short enough that a wedged mpv surfaces as an error rather
// than a frozen window.
const commandTimeout = 2 * time.Second

// stateInterval is how often playback state reaches the frontend.
//
// mpv reports time-pos at the video frame rate. Forwarding that would spend the
// WebView's budget redrawing a seek bar. 200 ms is five updates a second: smooth
// to the eye, and inside the 4-10/sec the design calls for.
const stateInterval = 200 * time.Millisecond

// connectTimeout bounds the wait for mpv to create its IPC pipe on startup.
const connectTimeout = 15 * time.Second

// MPVPlayer is a Player backed by an mpv child process.
type MPVPlayer struct {
	cfg Config
	log *logging.Logger

	proc   *process
	client *mpvipc.Client

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu       sync.RWMutex
	state    PlaybackState
	chapters []Chapter
	tracks   []Track

	// dirty marks that state changed since the last emit, so an idle player
	// costs nothing.
	dirty atomic.Bool

	// pump carries work from the IPC reader goroutine to the single goroutine
	// allowed to invoke callbacks.
	pump chan pumpMsg

	closeOnce sync.Once
	closed    atomic.Bool
}

// Compile-time proof that the implementation satisfies the interface.
var _ Player = (*MPVPlayer)(nil)

type pumpKind int

const (
	pumpTracks pumpKind = iota
	pumpFileLoaded
	pumpEndFile
	pumpError
)

type pumpMsg struct {
	kind pumpKind
	text string
}

// New starts mpv and connects to it.
//
// The returned player is ready to use; a failure here means mpv could not be
// started or did not open its IPC pipe, both of which are fatal to playback and
// are reported to the user rather than retried silently.
func New(ctx context.Context, cfg Config) (*MPVPlayer, error) {
	if cfg.MPVPath == "" {
		return nil, errors.New("player: no mpv executable was provided")
	}
	if cfg.Logger == nil {
		cfg.Logger = logging.New(logging.LevelInfo)
	}

	p := &MPVPlayer{
		cfg:  cfg,
		log:  cfg.Logger,
		pump: make(chan pumpMsg, 64),
		state: PlaybackState{
			Volume:        clampVolume(cfg.InitialVolume),
			Speed:         clampSpeed(cfg.InitialSpeed),
			Muted:         cfg.InitialMuted,
			SubtitleScale: 1,
			Paused: true,
			Idle:   true,

			ChapterIndex: -1,
			SubtitleID:   NoTrack,
			AudioID:      NoTrack,
		},
	}
	p.ctx, p.cancel = context.WithCancel(context.Background())

	pipeName := pipeNameFor()
	args := mpvArgs(cfg, pipeName)

	p.log.Info("player: starting mpv: %s", cfg.MPVPath)
	p.log.Debug("player: mpv arguments: %v", args)

	proc, err := startMPV(cfg.MPVPath, args)
	if err != nil {
		p.cancel()
		return nil, err
	}
	p.proc = proc

	dialCtx, cancelDial := context.WithTimeout(ctxOrBackground(ctx), connectTimeout)
	defer cancelDial()

	client, err := mpvipc.Dial(dialCtx, pipeName, p.handleEvent)
	if err != nil {
		proc.Stop()
		p.cancel()
		return nil, fmt.Errorf("player: connecting to mpv: %w", err)
	}
	p.client = client

	if err := p.observeProperties(dialCtx); err != nil {
		_ = client.Close()
		proc.Stop()
		p.cancel()
		return nil, err
	}

	p.wg.Add(2)
	go p.runPump()
	go p.watchProcess()

	p.log.Info("player: mpv is ready on %s", pipeName)
	return p, nil
}

// observedProperties are the mpv properties the player tracks.
//
// Every one is either shown in the UI or needed to keep the UI consistent;
// observing more would add event traffic for nothing.
var observedProperties = []string{
	"time-pos",
	"duration",
	"pause",
	"volume",
	"mute",
	"speed",
	"play-dir",
	"chapter",
	"chapter-list",
	"track-list",
	"media-title",
	"path",
	"eof-reached",
	"idle-active",
	"seeking",
	"sid",
	"aid",
	"sub-delay",
	"audio-delay",
	"sub-scale",
	"hwdec-current",
}

func (p *MPVPlayer) observeProperties(ctx context.Context) error {
	for i, name := range observedProperties {
		if err := p.client.ObserveProperty(ctx, int64(i+1), name); err != nil {
			return fmt.Errorf("player: observing %q: %w", name, err)
		}
	}
	return nil
}

// handleEvent runs on the IPC reader goroutine.
//
// It must not block and must not invoke user callbacks; it updates state and
// hands anything that needs a callback to the pump goroutine.
func (p *MPVPlayer) handleEvent(ev mpvipc.Event) {
	switch ev.Name {
	case "property-change":
		p.applyProperty(ev.Property, ev.Data)

	case "file-loaded":
		p.mu.Lock()
		p.state.FileLoaded = true
		p.state.EOF = false
		p.state.Idle = false
		path := p.state.Path
		p.mu.Unlock()
		p.dirty.Store(true)
		p.send(pumpMsg{kind: pumpFileLoaded, text: path})

	case "end-file":
		// Reported for the record only. With --keep-open, mpv does not emit this
		// when a file reaches its end, so the natural end is detected from the
		// eof-reached property instead; the reasons that do arrive here ("stop",
		// "redirect") all mean the file was replaced.
		p.log.Debug("player: end-file (%s)", endFileReason(ev.Raw))

	case "start-file":
		p.mu.Lock()
		p.state.EOF = false
		p.mu.Unlock()
		p.dirty.Store(true)

	default:
		// mpv emits many events we have no use for; ignoring them is correct,
		// and logging each one would drown the log.
	}
}

// send queues a message for the pump goroutine, dropping it rather than
// blocking the IPC reader if the queue is somehow full.
func (p *MPVPlayer) send(msg pumpMsg) {
	select {
	case p.pump <- msg:
	default:
		p.log.Warn("player: event queue is full; dropped a %v notification", msg.kind)
	}
}

// runPump is the only goroutine that invokes the configured callbacks. Keeping
// that in one place is what makes it safe to call into Wails from them.
func (p *MPVPlayer) runPump() {
	defer p.wg.Done()

	ticker := time.NewTicker(stateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return

		case msg := <-p.pump:
			p.deliver(msg)

		case <-ticker.C:
			if !p.dirty.Swap(false) {
				continue // nothing changed; an idle player costs nothing
			}
			if p.cfg.OnState != nil {
				p.cfg.OnState(p.State())
			}
		}
	}
}

func (p *MPVPlayer) deliver(msg pumpMsg) {
	switch msg.kind {
	case pumpTracks:
		if p.cfg.OnTracks != nil {
			p.cfg.OnTracks(p.Tracks(), p.Chapters())
		}
	case pumpFileLoaded:
		// Read mpv's real values before telling anyone the file is open, so the
		// first state the interface sees is accurate rather than assumed.
		p.syncFromMPV()
		if p.cfg.OnFileLoaded != nil {
			p.cfg.OnFileLoaded(msg.text)
		}
	case pumpEndFile:
		if p.cfg.OnEndFile != nil {
			p.cfg.OnEndFile(msg.text)
		}
	case pumpError:
		if p.cfg.OnError != nil {
			p.cfg.OnError(msg.text)
		}
	}
}

// watchProcess notices mpv dying and reports it as a user-facing failure rather
// than leaving the UI apparently working but inert.
func (p *MPVPlayer) watchProcess() {
	defer p.wg.Done()

	select {
	case <-p.ctx.Done():
		return

	case <-p.proc.Done():
		if p.closed.Load() {
			return // an expected exit during shutdown
		}
		err := p.proc.Err()
		p.log.Error("player: mpv exited unexpectedly: %v", err)
		p.send(pumpMsg{
			kind: pumpError,
			text: "The media engine (mpv) stopped unexpectedly. " +
				"This usually means the file is severely damaged. " +
				"Restart PlayerOne to continue.",
		})

	case <-p.client.Done():
		if p.closed.Load() {
			return
		}
		p.log.Error("player: lost the connection to mpv: %v", p.client.Err())
		p.send(pumpMsg{
			kind: pumpError,
			text: "Lost the connection to the media engine (mpv). Restart PlayerOne to continue.",
		})
	}
}

// command issues an mpv command with the standard timeout.
func (p *MPVPlayer) command(ctx context.Context, args ...any) error {
	if p.closed.Load() {
		return errors.New("player: the player is closed")
	}

	ctx, cancel := context.WithTimeout(ctxOrBackground(ctx), commandTimeout)
	defer cancel()

	_, err := p.client.Command(ctx, args...)
	if err != nil {
		p.log.Debug("player: command %v failed: %v", args, err)
	}
	return err
}

func (p *MPVPlayer) setProperty(ctx context.Context, name string, value any) error {
	if err := p.command(ctx, "set_property", name, value); err != nil {
		return fmt.Errorf("player: setting %s: %w", name, err)
	}
	return nil
}

// Load opens a file.
func (p *MPVPlayer) Load(ctx context.Context, path string) error {
	if path == "" {
		return errors.New("player: no file path was given")
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path // a path we cannot absolutise is still worth trying
	}

	p.mu.Lock()
	p.state = PlaybackState{
		// Carry across the settings that belong to the session rather than the
		// file, so opening a video does not reset the user's volume or speed.
		Volume: p.state.Volume,
		Muted:  p.state.Muted,
		Speed:  p.state.Speed,

		// Paused carries across too. mpv only emits a property-change when a
		// value actually changes, so assuming "paused" here would leave the
		// state wrong for the whole session: mpv starts the new file playing,
		// its pause property never changes, and no event ever corrects us.
		// syncFromMPV, run once the file is open, is what makes this exact.
		Paused: p.state.Paused,

		Path:         abs,
		Title:        filepath.Base(abs),
		ChapterIndex: -1,
		SubtitleID:   NoTrack,
		AudioID:      NoTrack,
	}
	p.chapters = nil
	p.tracks = nil
	p.mu.Unlock()

	p.dirty.Store(true)

	p.log.Info("player: loading %s", abs)
	if err := p.command(ctx, "loadfile", abs, "replace"); err != nil {
		return fmt.Errorf("player: loading %s: %w", abs, err)
	}
	return nil
}

// Unload closes the current file.
func (p *MPVPlayer) Unload(ctx context.Context) error {
	if err := p.command(ctx, "stop"); err != nil {
		return fmt.Errorf("player: stopping playback: %w", err)
	}

	p.mu.Lock()
	p.state.FileLoaded = false
	p.state.Idle = true
	p.state.Path = ""
	p.state.Title = ""
	p.state.Position = 0
	p.state.Duration = 0
	p.state.ChapterIndex = -1
	p.chapters = nil
	p.tracks = nil
	p.mu.Unlock()

	p.dirty.Store(true)
	p.send(pumpMsg{kind: pumpTracks})
	return nil
}

func (p *MPVPlayer) Play(ctx context.Context) error {
	return p.setProperty(ctx, "pause", false)
}

func (p *MPVPlayer) Pause(ctx context.Context) error {
	return p.setProperty(ctx, "pause", true)
}

func (p *MPVPlayer) TogglePause(ctx context.Context) error {
	// Toggled by mpv rather than by reading the state and inverting it, which
	// would race with a pause arriving from elsewhere.
	if err := p.command(ctx, "cycle", "pause"); err != nil {
		return fmt.Errorf("player: toggling pause: %w", err)
	}
	return nil
}

// Seek moves to an absolute position.
func (p *MPVPlayer) Seek(ctx context.Context, seconds float64) error {
	if seconds < 0 {
		seconds = 0
	}
	if err := p.command(ctx, "seek", mpvipc.Float(seconds), "absolute+exact"); err != nil {
		return fmt.Errorf("player: seeking to %.3fs: %w", seconds, err)
	}
	return nil
}

// SeekRelative moves by an offset.
func (p *MPVPlayer) SeekRelative(ctx context.Context, delta float64) error {
	if err := p.command(ctx, "seek", mpvipc.Float(delta), "relative+exact"); err != nil {
		return fmt.Errorf("player: seeking by %.3fs: %w", delta, err)
	}
	return nil
}

// SeekChapter jumps to a chapter by index.
func (p *MPVPlayer) SeekChapter(ctx context.Context, index int) error {
	p.mu.RLock()
	chapters := p.chapters
	p.mu.RUnlock()

	if index < 0 || index >= len(chapters) {
		return fmt.Errorf("player: chapter %d does not exist (this file has %d)", index, len(chapters))
	}

	// Seeking to the chapter's start time rather than setting mpv's chapter
	// property gives identical results and keeps one seek path in the code.
	return p.Seek(ctx, chapters[index].Start)
}

func (p *MPVPlayer) SetVolume(ctx context.Context, value float64) error {
	// mpvipc.Float, not a bare float64: mpv rejects an integer for its
	// floating-point properties, and Go writes a whole number without a
	// decimal point. See mpvipc.Float.
	return p.setProperty(ctx, "volume", mpvipc.Float(clampVolume(value)))
}

func (p *MPVPlayer) SetMute(ctx context.Context, muted bool) error {
	return p.setProperty(ctx, "mute", muted)
}

func (p *MPVPlayer) SetSpeed(ctx context.Context, speed float64) error {
	return p.setProperty(ctx, "speed", mpvipc.Float(clampSpeed(speed)))
}

// SetReverse switches playback direction.
//
// Backward playback is a genuine mpv feature rather than repeated backward
// seeking, so the picture stays smooth. mpv documents it as demanding: it
// re-decodes from keyframes in reverse, so it is CPU-heavy and needs the
// demuxer history that mpvArgs reserves.
func (p *MPVPlayer) SetReverse(ctx context.Context, reverse bool) error {
	direction := "forward"
	if reverse {
		direction = "backward"
	}

	if err := p.setProperty(ctx, "play-dir", direction); err != nil {
		return fmt.Errorf("player: switching to %s playback: %w", direction, err)
	}

	// mpv accepts the change but does not emit a property-change for play-dir,
	// so nothing would ever tell us the direction had flipped. Without this
	// read-back the state stays "forward" for ever and the toggle keeps sending
	// "backward", leaving no way back.
	p.refreshProperty(ctx, "play-dir")
	return nil
}

// refreshProperty reads one property and folds it into the state.
//
// For the handful of properties mpv does not report changes for, this is what
// keeps the state honest: it reflects what mpv actually holds rather than what
// we asked for.
func (p *MPVPlayer) refreshProperty(ctx context.Context, name string) {
	readCtx, cancel := context.WithTimeout(ctxOrBackground(ctx), commandTimeout)
	defer cancel()

	data, err := p.client.Command(readCtx, "get_property", name)
	if err != nil {
		p.log.Debug("player: could not read %q back: %v", name, err)
		return
	}
	p.applyProperty(name, data)
}

func (p *MPVPlayer) SetSubtitleTrack(ctx context.Context, id int) error {
	if id <= NoTrack {
		return p.DisableSubtitles(ctx)
	}
	return p.setProperty(ctx, "sid", id)
}

func (p *MPVPlayer) DisableSubtitles(ctx context.Context) error {
	// mpv spells "no track" as the string "no" on sid and aid.
	return p.setProperty(ctx, "sid", "no")
}

func (p *MPVPlayer) SetAudioTrack(ctx context.Context, id int) error {
	if id <= NoTrack {
		return p.setProperty(ctx, "aid", "no")
	}
	return p.setProperty(ctx, "aid", id)
}

// AddSubtitleFile attaches an external subtitle file and selects it.
func (p *MPVPlayer) AddSubtitleFile(ctx context.Context, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	if err := p.command(ctx, "sub-add", abs, "select"); err != nil {
		return fmt.Errorf("player: adding the subtitle file %s: %w", filepath.Base(abs), err)
	}
	p.log.Info("player: attached the subtitle file %s", abs)
	return nil
}

func (p *MPVPlayer) SetSubtitleDelay(ctx context.Context, seconds float64) error {
	return p.setProperty(ctx, "sub-delay", mpvipc.Float(seconds))
}

func (p *MPVPlayer) SetAudioDelay(ctx context.Context, seconds float64) error {
	return p.setProperty(ctx, "audio-delay", mpvipc.Float(seconds))
}

// SetSubtitleScale resizes subtitles, 1 being mpv's own choice.
//
// sub-scale rather than sub-font-size: it is a multiplier, so it keeps its
// meaning across files whose subtitles specify their own sizes, and it applies
// to styled ASS subtitles as well as plain text.
func (p *MPVPlayer) SetSubtitleScale(ctx context.Context, scale float64) error {
	return p.setProperty(ctx, "sub-scale", mpvipc.Float(clampSubtitleScale(scale)))
}

// State returns the current snapshot.
func (p *MPVPlayer) State() PlaybackState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

// Chapters returns a copy of the chapter list.
func (p *MPVPlayer) Chapters() []Chapter {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]Chapter(nil), p.chapters...)
}

// Tracks returns a copy of the track list.
func (p *MPVPlayer) Tracks() []Track {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]Track(nil), p.tracks...)
}

// Close shuts mpv down and waits for every goroutine to finish.
func (p *MPVPlayer) Close() error {
	var err error

	p.closeOnce.Do(func() {
		p.closed.Store(true)
		p.log.Info("player: shutting down")

		// Ask mpv to exit cleanly first so it can flush its own state; the
		// process kill below is the fallback for an mpv that will not go.
		if p.client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			_, _ = p.client.Command(ctx, "quit")
			cancel()
			_ = p.client.Close()
		}

		p.cancel()
		p.wg.Wait()

		if p.proc != nil {
			select {
			case <-p.proc.Done():
			case <-time.After(2 * time.Second):
				p.log.Warn("player: mpv did not exit on request; terminating it")
				p.proc.Stop()
				<-p.proc.Done()
			}
		}

		p.log.Info("player: shutdown complete")
	})

	return err
}
