package tools

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// touchExe creates an empty file named as an executable would be on this OS.
func touchExe(t *testing.T, dir, name string) string {
	t.Helper()
	filename := name
	if runtime.GOOS == "windows" {
		filename += ".exe"
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte("stub"), 0o755); err != nil {
		t.Fatalf("writing stub %s: %v", path, err)
	}
	return path
}

func TestLookFindsToolInSearchDir(t *testing.T) {
	dir := t.TempDir()
	want := touchExe(t, dir, MPV)

	r := NewResolverWithDirs([]string{dir})
	got, err := r.Look(MPV)
	if err != nil {
		t.Fatalf("Look(mpv) returned error: %v", err)
	}
	if !strings.EqualFold(got, want) {
		t.Errorf("Look(mpv) = %q, want %q", got, want)
	}
}

func TestLookPrefersEarlierSearchDir(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	wantPath := touchExe(t, first, FFmpeg)
	touchExe(t, second, FFmpeg)

	r := NewResolverWithDirs([]string{first, second})
	got, err := r.Look(FFmpeg)
	if err != nil {
		t.Fatalf("Look(ffmpeg) returned error: %v", err)
	}
	if !strings.EqualFold(got, wantPath) {
		t.Errorf("Look(ffmpeg) = %q, want the first search dir %q", got, wantPath)
	}
}

func TestLookReturnsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	touchExe(t, dir, MPV)

	r := NewResolverWithDirs([]string{dir})
	got, err := r.Look(MPV)
	if err != nil {
		t.Fatalf("Look(mpv) returned error: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("Look(mpv) = %q, want an absolute path", got)
	}
}

func TestLookIgnoresDirectoriesNamedLikeTheTool(t *testing.T) {
	dir := t.TempDir()
	name := MPV
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatalf("creating decoy directory: %v", err)
	}

	// Only a decoy directory exists, and PATH on a test machine will not have a
	// tool by this deliberately unused name.
	r := NewResolverWithDirs([]string{dir})
	if _, err := r.Look("playerone-nonexistent-tool"); err == nil {
		t.Fatal("expected an error for a tool that does not exist")
	}
}

func TestLookMissingToolReportsWhereItSearched(t *testing.T) {
	dir := t.TempDir()
	r := NewResolverWithDirs([]string{dir})

	_, err := r.Look("playerone-definitely-not-a-real-tool")
	if err == nil {
		t.Fatal("expected an error for a missing tool")
	}

	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("error is %T, want *NotFoundError", err)
	}
	if !strings.Contains(nf.Searched, dir) {
		t.Errorf("Searched = %q, want it to mention %q", nf.Searched, dir)
	}
	if !strings.Contains(nf.Searched, "PATH") {
		t.Errorf("Searched = %q, want it to mention PATH", nf.Searched)
	}
}

func TestNotFoundFriendlyMessageIsActionable(t *testing.T) {
	for _, tool := range []string{MPV, FFmpeg, FFprobe} {
		e := &NotFoundError{Tool: tool, Searched: "\n  - somewhere"}
		msg := e.FriendlyMessage()
		if !strings.Contains(msg, "Fix:") {
			t.Errorf("FriendlyMessage for %s does not tell the user what to do: %q", tool, msg)
		}
		if !strings.Contains(strings.ToLower(msg), tool) {
			t.Errorf("FriendlyMessage for %s does not name the tool: %q", tool, msg)
		}
	}

	// Only mpv is fatal; the optional tools must say playback still works.
	for _, tool := range []string{FFmpeg, FFprobe} {
		msg := (&NotFoundError{Tool: tool}).FriendlyMessage()
		if !strings.Contains(msg, "still works") {
			t.Errorf("%s is optional, but its message does not say so: %q", tool, msg)
		}
	}
}

func TestAvailable(t *testing.T) {
	dir := t.TempDir()
	touchExe(t, dir, FFprobe)
	r := NewResolverWithDirs([]string{dir})

	if !r.Available(FFprobe) {
		t.Error("Available(ffprobe) = false, want true")
	}
	if r.Available("playerone-definitely-not-a-real-tool") {
		t.Error("Available(nonexistent) = true, want false")
	}
}

// A second Look must not re-hit the filesystem, and must agree with the first.
func TestLookIsCached(t *testing.T) {
	dir := t.TempDir()
	path := touchExe(t, dir, MPV)

	r := NewResolverWithDirs([]string{dir})
	first, err := r.Look(MPV)
	if err != nil {
		t.Fatalf("first Look: %v", err)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("removing stub: %v", err)
	}

	second, err := r.Look(MPV)
	if err != nil {
		t.Fatalf("second Look after cache should still succeed: %v", err)
	}
	if first != second {
		t.Errorf("cached Look returned %q, first returned %q", second, first)
	}
}

func TestNewResolverIncludesLocalBinDirs(t *testing.T) {
	r := NewResolver()
	dirs := r.SearchDirs()
	if len(dirs) == 0 {
		t.Fatal("NewResolver produced no search directories")
	}

	var sawBin bool
	for _, d := range dirs {
		if filepath.Base(d) == "bin" {
			sawBin = true
		}
		if !filepath.IsAbs(d) {
			t.Errorf("search dir %q is not absolute", d)
		}
	}
	if !sawBin {
		t.Errorf("search dirs %v contain no bin directory", dirs)
	}
}
