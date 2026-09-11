//go:build windows

package updater

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
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
// installer cannot replace files that are still in use.
//
// ShellExecute rather than CreateProcess, because the installer asks for the
// highest rights the account has and its manifest says so. CreateProcess -
// which is what os/exec uses - refuses such a program outright with "the
// requested operation requires elevation"; it has no way to ask. Only the
// shell can raise the User Account Control prompt, which is what happens when
// the installer is started from Explorer. The process it starts is independent
// of this one, so there is nothing to detach.
//
// The verb is deliberately the default one and not "runas". The installer can
// be a for-me-only copy living in the user's own AppData, and "runas" on a
// standard account asks for an administrator's password and then runs as that
// administrator - updating their profile rather than the one that asked. The
// manifest already requests the highest token available, so an administrator
// still gets the UAC prompt needed for Program Files, and a standard user
// updating their private copy is not asked for a password they do not have.
func Launch(installerPath string) error {
	if !strings.EqualFold(filepath.Ext(installerPath), ".exe") {
		return fmt.Errorf("updater: %s is not an installer", filepath.Base(installerPath))
	}
	if _, err := os.Stat(installerPath); err != nil {
		return fmt.Errorf("updater: the downloaded installer is missing: %w", err)
	}

	// An absolute path, so the shell cannot resolve the name against a search
	// path and start something else.
	path, err := filepath.Abs(installerPath)
	if err != nil {
		return fmt.Errorf("updater: resolving the installer path: %w", err)
	}

	file, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("updater: preparing the installer command: %w", err)
	}
	dir, err := syscall.UTF16PtrFromString(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("updater: preparing the installer command: %w", err)
	}

	// The shell is a COM service, and this runs on whichever thread the
	// interface call landed on rather than the initialised main one.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	switch err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); {
	case err == nil, errors.Is(err, syscall.Errno(sFalse)):
		defer windows.CoUninitialize()
	}

	if err := windows.ShellExecute(0, nil, file, nil, dir, windows.SW_SHOWNORMAL); err != nil {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			// Declining the prompt is a decision, not a fault. PlayerOne stays
			// open on the version it is already running.
			return errors.New("the update needs your permission to install, and that was declined")
		}
		return fmt.Errorf("updater: starting the installer: %w", err)
	}
	return nil
}

// sFalse is COM's "already initialised on this thread", which is a success.
const sFalse = 1
