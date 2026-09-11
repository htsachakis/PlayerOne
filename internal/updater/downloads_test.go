package updater

import (
	"os"
	"path/filepath"
	"testing"
)

// useTempDir points os.TempDir at a directory of this test's own, so a sweep
// cannot reach the real one.
func useTempDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("TMPDIR", dir) // unix
	t.Setenv("TMP", dir)    // windows
	t.Setenv("TEMP", dir)

	if got := os.TempDir(); got != dir {
		t.Fatalf("os.TempDir() = %q, want the test's %q", got, dir)
	}
	return dir
}

func TestCleanDownloadDirsRemovesWhatTheUpdaterLeaves(t *testing.T) {
	useTempDir(t)

	first, err := NewDownloadDir()
	if err != nil {
		t.Fatalf("NewDownloadDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(first, "installer.exe"), []byte("MZ"), 0o600); err != nil {
		t.Fatalf("writing the fake installer: %v", err)
	}

	second, err := NewDownloadDir()
	if err != nil {
		t.Fatalf("NewDownloadDir: %v", err)
	}

	removed, err := CleanDownloadDirs()
	if err != nil {
		t.Fatalf("CleanDownloadDirs: %v", err)
	}
	if removed != 2 {
		t.Errorf("removed %d folders, want 2", removed)
	}

	for _, dir := range []string{first, second} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("%s is still there", dir)
		}
	}
}

// The sweep runs in a directory shared with the rest of the system, so what it
// does NOT touch matters as much as what it does.
func TestCleanDownloadDirsLeavesEverythingElseAlone(t *testing.T) {
	tmp := useTempDir(t)

	strangers := []string{
		"someone-elses-folder",
		"playerone",         // ours, but not a download
		"playerone-notes-1", // a different prefix that starts the same way
	}
	for _, name := range strangers {
		if err := os.Mkdir(filepath.Join(tmp, name), 0o755); err != nil {
			t.Fatalf("creating %s: %v", name, err)
		}
	}

	// A file, not a folder, whose name would otherwise match.
	stray := filepath.Join(tmp, downloadPrefix+"stray.exe")
	if err := os.WriteFile(stray, []byte("MZ"), 0o600); err != nil {
		t.Fatalf("writing %s: %v", stray, err)
	}

	if _, err := CleanDownloadDirs(); err != nil {
		t.Fatalf("CleanDownloadDirs: %v", err)
	}

	for _, name := range strangers {
		if _, err := os.Stat(filepath.Join(tmp, name)); err != nil {
			t.Errorf("%s was removed: %v", name, err)
		}
	}
	if _, err := os.Stat(stray); err != nil {
		t.Errorf("the stray file was removed: %v", err)
	}
}

func TestCleanDownloadDirsOnAnEmptyTempDir(t *testing.T) {
	useTempDir(t)

	removed, err := CleanDownloadDirs()
	if err != nil {
		t.Fatalf("CleanDownloadDirs: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed %d folders from an empty directory, want 0", removed)
	}
}
