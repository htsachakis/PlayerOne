//go:build windows

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// IsInstalled reports whether this copy was put here by the installer.
//
// The uninstaller sits beside the executable in an installed copy and is absent
// from a portable one. The distinction matters: running the installer over a
// portable folder would silently convert it into an installed application
// somewhere else on disk, which is not what "update" should mean.
func IsInstalled() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	for _, name := range []string{"uninstall.exe", "Uninstall.exe"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(exe), name)); err == nil {
			return true
		}
	}
	return false
}

// Launch starts the downloaded installer and returns.
//
// The caller is expected to shut PlayerOne down immediately afterwards: the
// installer cannot replace files that are still in use. It is started detached
// so that it outlives this process.
func Launch(installerPath string) error {
	if !strings.EqualFold(filepath.Ext(installerPath), ".exe") {
		return fmt.Errorf("updater: %s is not an installer", filepath.Base(installerPath))
	}
	if _, err := os.Stat(installerPath); err != nil {
		return fmt.Errorf("updater: the downloaded installer is missing: %w", err)
	}

	cmd := exec.Command(installerPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// Detached, with its own process group, so closing PlayerOne does not
		// take the installer down with it.
		CreationFlags: 0x00000008 | 0x00000200, // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("updater: starting the installer: %w", err)
	}

	// Not waited on deliberately: this process is about to exit so that the
	// installer can replace it.
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("updater: releasing the installer process: %w", err)
	}
	return nil
}
