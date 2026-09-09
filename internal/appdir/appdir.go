// Package appdir resolves where PlayerOne keeps its state.
//
// State never lives beside the executable: an installed copy under Program Files
// is not writable, and a portable copy on a USB stick should not accumulate one
// user's history.
package appdir

import (
	"fmt"
	"os"
	"path/filepath"

	"playerone/internal/branding"
)

// Config returns the per-user configuration directory, creating it if needed.
//
// On Windows this is %APPDATA%\PlayerOne.
func Config() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("appdir: locating the user configuration directory: %w", err)
	}

	dir := filepath.Join(base, branding.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("appdir: creating %s: %w", dir, err)
	}
	return dir, nil
}

// WriteFileAtomic writes data to path via a temporary file in the same
// directory followed by a rename.
//
// Settings and history are rewritten on a timer and at shutdown, so a partial
// write during a crash or power loss is a realistic way to lose them. A rename
// on the same volume is atomic, so the file on disk is always either the old
// contents or the new ones.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("appdir: creating %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("appdir: creating a temporary file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()

	// Any failure past this point must not leave the temporary file behind.
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("appdir: writing %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("appdir: flushing %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("appdir: closing %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("appdir: setting permissions on %s: %w", tmpName, err)
	}

	// Windows will not rename onto an existing file, so clear the way first.
	// A crash in this window loses the file, which the caller recovers from by
	// falling back to defaults - strictly better than a truncated file that
	// parses into nonsense.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("appdir: replacing %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("appdir: renaming %s to %s: %w", tmpName, path, err)
	}

	tmpName = "" // renamed successfully; nothing to clean up
	return nil
}
