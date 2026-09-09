// Package tools locates the external binaries PlayerOne drives: mpv (required)
// and ffmpeg/ffprobe (optional).
//
// Resolution deliberately prefers a bin/ directory next to the application over
// PATH, so a bundled, known-good mpv wins over whatever the user happens to have
// installed. No developer-machine path is ever baked in.
package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Tool names, used as both the lookup key and the base filename.
const (
	MPV     = "mpv"
	FFmpeg  = "ffmpeg"
	FFprobe = "ffprobe"
)

// Resolver finds tool executables and caches the results.
//
// Lookups are cached because they hit the filesystem and are called from the
// media pipeline; a tool appearing or disappearing mid-session is not a case
// worth re-checking on every subtitle extraction.
type Resolver struct {
	mu    sync.Mutex
	dirs  []string
	cache map[string]string
}

// NewResolver builds a resolver searching, in order:
//
//  1. <dir containing the running executable>/bin
//  2. <dir containing the running executable>
//  3. <working directory>/bin   — this is the one `wails dev` needs
//
// ...then PATH. Directories that do not exist are harmless; they simply never
// match.
func NewResolver() *Resolver {
	var dirs []string

	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		exeDir := filepath.Dir(exe)
		dirs = append(dirs, filepath.Join(exeDir, "bin"), exeDir)
	}

	// `wails dev` builds into a temporary location, so the executable directory
	// is not the project. The working directory is.
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(wd, "bin"))
	}

	return NewResolverWithDirs(dirs)
}

// NewResolverWithDirs builds a resolver over an explicit search path. Tests use
// it; production code wants NewResolver.
func NewResolverWithDirs(dirs []string) *Resolver {
	return &Resolver{
		dirs:  append([]string(nil), dirs...),
		cache: make(map[string]string),
	}
}

// SearchDirs returns the local directories consulted before PATH. Exposed for
// the Info/diagnostics view so "mpv not found" can say where we looked.
func (r *Resolver) SearchDirs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.dirs...)
}

// Look returns the absolute path to a tool.
//
// The returned error is written for a user, not a developer: it names the tool,
// where we searched, and what to do about it.
func (r *Resolver) Look(name string) (string, error) {
	r.mu.Lock()
	if path, ok := r.cache[name]; ok {
		r.mu.Unlock()
		if path == "" {
			return "", r.notFound(name)
		}
		return path, nil
	}
	dirs := append([]string(nil), r.dirs...)
	r.mu.Unlock()

	path := searchDirs(dirs, name)
	if path == "" {
		// PATH is the fallback, so a system-wide install still works.
		if p, err := exec.LookPath(name); err == nil {
			path, _ = filepath.Abs(p)
		}
	}

	r.mu.Lock()
	r.cache[name] = path
	r.mu.Unlock()

	if path == "" {
		return "", r.notFound(name)
	}
	return path, nil
}

// Available reports whether a tool can be found, without producing an error.
// Used for the optional tools, where absence is a degraded mode rather than a
// failure.
func (r *Resolver) Available(name string) bool {
	_, err := r.Look(name)
	return err == nil
}

func searchDirs(dirs []string, name string) string {
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		for _, candidate := range executableNames(name) {
			full := filepath.Join(dir, candidate)
			info, err := os.Stat(full)
			if err != nil || info.IsDir() {
				continue
			}
			if abs, err := filepath.Abs(full); err == nil {
				return abs
			}
			return full
		}
	}
	return ""
}

// executableNames lists the filenames a tool may have on this platform.
func executableNames(name string) []string {
	if runtime.GOOS == "windows" {
		return []string{name + ".exe"}
	}
	return []string{name}
}

func (r *Resolver) notFound(name string) error {
	r.mu.Lock()
	dirs := append([]string(nil), r.dirs...)
	r.mu.Unlock()

	var where strings.Builder
	for _, d := range dirs {
		if d == "" {
			continue
		}
		where.WriteString("\n  - ")
		where.WriteString(d)
	}
	where.WriteString("\n  - directories on PATH")

	return &NotFoundError{Tool: name, Searched: where.String()}
}

// NotFoundError reports a missing external tool with enough context for the UI
// to render an actionable message.
type NotFoundError struct {
	Tool     string
	Searched string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("could not find %q. Searched:%s", e.Tool, e.Searched)
}

// FriendlyMessage is the text shown to a user, as opposed to the log.
func (e *NotFoundError) FriendlyMessage() string {
	switch e.Tool {
	case MPV:
		return "mpv was not found. PlayerOne uses mpv to decode and display video.\n\n" +
			"Fix: download the Windows build of mpv and place mpv.exe in the bin folder " +
			"next to PlayerOne.exe, or install mpv so that it is on your PATH."
	case FFmpeg:
		return "ffmpeg was not found. Playback still works, but transcripts cannot be " +
			"extracted from subtitle tracks embedded in the video.\n\n" +
			"Fix: place ffmpeg.exe in the bin folder next to PlayerOne.exe, or install " +
			"ffmpeg so that it is on your PATH."
	case FFprobe:
		return "ffprobe was not found. Playback still works, but the Info tab will show " +
			"less detail.\n\n" +
			"Fix: place ffprobe.exe in the bin folder next to PlayerOne.exe, or install " +
			"ffmpeg so that it is on your PATH."
	default:
		return e.Error()
	}
}
