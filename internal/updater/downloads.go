package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// downloadPrefix names the temporary folder one update download lives in.
//
// Both the folder that is created and the sweep that removes it are built from
// this, so the two cannot drift apart and start leaving files behind.
const downloadPrefix = "playerone-update-"

// NewDownloadDir makes a private folder to download one installer into.
//
// A folder of its own, rather than the temporary directory itself: the
// installer is executed from where it lands, and a folder nothing else writes
// to means nothing else can substitute the file between the checksum and the
// run.
func NewDownloadDir() (string, error) {
	dir, err := os.MkdirTemp("", downloadPrefix)
	if err != nil {
		return "", fmt.Errorf("updater: could not prepare a download folder: %w", err)
	}
	return dir, nil
}

// CleanDownloadDirs removes the folders earlier updates downloaded into, and
// reports how many went.
//
// The process that starts an installer cannot tidy up after it: PlayerOne has
// to quit so its own files can be replaced, and by the time the installer has
// finished there is nobody left to delete anything. The copy that comes back
// afterwards does it instead - which also catches the cases where no install
// ever happened, because the download was abandoned, the installer cancelled
// or the elevation prompt declined, and a 180 MB file would otherwise sit in
// the temporary directory until Windows got round to it.
//
// A folder still in use is left where it is. That is the installer that
// started this copy, still exiting; the next run picks it up.
func CleanDownloadDirs() (int, error) {
	tmp := os.TempDir()

	entries, err := os.ReadDir(tmp)
	if err != nil {
		return 0, fmt.Errorf("updater: reading %s: %w", tmp, err)
	}

	removed := 0
	var failed []string

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), downloadPrefix) {
			continue
		}

		// Rebuilt from the directory being walked rather than taken from the
		// entry, so a name cannot point anywhere but inside it.
		path := filepath.Join(tmp, filepath.Base(entry.Name()))
		if err := os.RemoveAll(path); err != nil {
			failed = append(failed, entry.Name())
			continue
		}
		removed++
	}

	if len(failed) > 0 {
		return removed, fmt.Errorf("updater: %s still in use", strings.Join(failed, ", "))
	}
	return removed, nil
}
