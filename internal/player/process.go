package player

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"

	"playerone/internal/branding"
)

// mpvArgs builds mpv's command line.
//
// Every flag here exists for a reason recorded beside it. mpv's defaults are
// otherwise left alone deliberately: overriding decoder or output settings is
// how a player ends up working on the developer's GPU and nowhere else.
func mpvArgs(cfg Config, pipeName string) []string {
	args := []string{
		// Stay alive with no file open, so one mpv process serves the whole
		// session and the video window is never re-parented.
		"--idle=yes",

		// Ignore the user's mpv.conf and input.conf. A stray setting in a
		// personal config would otherwise change how this application behaves
		// in ways neither we nor the user could reasonably diagnose.
		"--no-config",

		"--input-ipc-server=" + pipeName,

		// mpv must draw no interface of its own: PlayerOne provides all of it,
		// and an mpv OSD drawn over the video could not be styled to match.
		"--no-osc",
		"--osd-level=0",
		"--no-terminal",

		// All keyboard handling belongs to the frontend. Without this, keys
		// would fire twice whenever the video window had focus.
		"--no-input-default-bindings",
		"--input-vo-keyboard=no",
		"--input-cursor=no",

		// Pause on the last frame instead of unloading, so the final image
		// stays on screen and the position remains readable at the end.
		"--keep-open=yes",
		"--keep-open-pause=yes",

		// Exact seeking. Chapter and transcript clicks must land on the
		// requested moment, not on the nearest keyframe several seconds away.
		"--hr-seek=yes",

		// Pick up "video.en.srt" beside "video.mkv" automatically. This is the
		// normal shape of a downloaded tutorial and its subtitles.
		"--sub-auto=fuzzy",
		"--audio-file-auto=fuzzy",

		// The video is a native window, so a file dropped on it never reaches
		// the WebView and Wails cannot see it. mpv accepts the drop instead;
		// "replace" rather than "append" keeps it to one file at a time, which
		// is the model the rest of the application works to. The backend
		// notices the unexpected file and reconciles history and the playlist.
		"--drag-and-drop=replace",

		// Let mpv choose the decoder. auto-safe avoids hardware paths known to
		// be broken, which is what makes one build work across NVIDIA, AMD and
		// Intel without per-vendor special cases.
		"--hwdec=auto-safe",

		// Keep speech intelligible at 2x and beyond, which is the whole point
		// of speed control on a tutorial.
		"--audio-pitch-correction=yes",

		// Backward playback needs demuxer history to reverse through. These are
		// caps rather than allocations, so they cost nothing until reverse is
		// actually used.
		"--demuxer-max-bytes=256MiB",
		"--demuxer-max-back-bytes=256MiB",

		// Quiet by default; raised to debug level by the caller when needed.
		"--msg-level=all=warn",

		"--volume=" + strconv.FormatFloat(clampVolume(cfg.InitialVolume), 'f', -1, 64),
		"--speed=" + strconv.FormatFloat(clampSpeed(cfg.InitialSpeed), 'f', -1, 64),
	}

	if cfg.InitialMuted {
		args = append(args, "--mute=yes")
	}

	if cfg.WindowID != 0 {
		// The embedding itself: mpv renders into our child window rather than
		// creating a top-level one of its own.
		args = append(args, "--wid="+strconv.FormatUint(uint64(cfg.WindowID), 10))
	} else {
		// Only reached when running the player outside the Wails shell.
		args = append(args, "--force-window=yes")
	}

	return args
}

// process supervises the mpv child process.
type process struct {
	cmd  *exec.Cmd
	once sync.Once
	done chan struct{}
	err  error
}

// startMPV spawns mpv.
//
// Arguments are passed as an argv array rather than a command string, so paths
// containing spaces, quotes or Unicode need no escaping and no shell is
// involved at any point.
func startMPV(mpvPath string, args []string) (*process, error) {
	cmd := exec.Command(mpvPath, args...)
	configureProcAttr(cmd)

	// mpv is told --no-terminal, but a pipe on stdout/stderr still prevents it
	// from inheriting a console it might write to.
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("player: starting mpv (%s): %w", mpvPath, err)
	}

	p := &process{cmd: cmd, done: make(chan struct{})}
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()

	return p, nil
}

// Done is closed when mpv exits.
func (p *process) Done() <-chan struct{} { return p.done }

// Err reports how mpv exited, once Done is closed.
func (p *process) Err() error { return p.err }

// Stop asks mpv to exit and, failing that, kills it.
//
// mpv is asked to quit over IPC first by the caller; this is the fallback for a
// process that is wedged or has lost its IPC connection.
func (p *process) Stop() {
	p.once.Do(func() {
		if p.cmd.Process == nil {
			return
		}
		_ = p.cmd.Process.Kill()
	})
}

// pipeNameFor builds a per-process IPC pipe name so two running copies of
// PlayerOne cannot collide.
func pipeNameFor() string {
	return branding.IPCPipePrefix + strconv.Itoa(os.Getpid())
}

// Bounds mirroring internal/settings, applied here too because mpv is the last
// line of defence against a nonsensical value reaching the decoder.
const (
	minSpeed = 0.1
	maxSpeed = 16.0
)

func clampSpeed(v float64) float64 {
	if !(v >= minSpeed) { // also catches NaN
		return 1.0
	}
	if v > maxSpeed {
		return maxSpeed
	}
	return v
}

// Subtitle scale bounds. Below a quarter the text is unreadable; above four
// times it fills the frame, and both are certainly a mis-click rather than a
// preference.
const (
	minSubtitleScale = 0.25
	maxSubtitleScale = 4.0
)

func clampSubtitleScale(v float64) float64 {
	if !(v >= minSubtitleScale) { // also catches NaN
		return 1.0
	}
	if v > maxSubtitleScale {
		return maxSubtitleScale
	}
	return v
}

func clampVolume(v float64) float64 {
	if !(v >= 0) {
		return 100
	}
	if v > 150 {
		return 150
	}
	return v
}

// ctxOrBackground keeps callers from having to pass a context they do not have.
func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
