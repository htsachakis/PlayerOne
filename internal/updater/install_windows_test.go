//go:build windows

package updater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Launch hands a path to the Windows shell, which will run whatever it is
// given. These guards are the whole of what stands between that and a download
// that turned out to be something other than an installer.
func TestLaunchRefusesAnythingButAnExe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "installer.txt")
	if err := os.WriteFile(path, []byte("not an installer"), 0o644); err != nil {
		t.Fatalf("writing: %v", err)
	}

	err := Launch(path)
	if err == nil {
		t.Fatal("expected a refusal for a file that is not an executable")
	}
	if !strings.Contains(err.Error(), "not an installer") {
		t.Errorf("error = %v", err)
	}
}

func TestLaunchRefusesAMissingFile(t *testing.T) {
	err := Launch(filepath.Join(t.TempDir(), "absent.exe"))
	if err == nil {
		t.Fatal("expected an error for a file that is not there")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error = %v", err)
	}
}
