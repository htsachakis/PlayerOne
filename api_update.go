package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"playerone/internal/updater"
	"playerone/internal/version"
)

// Update event names.
const (
	eventUpdateAvailable = "update:available"
	eventUpdateProgress  = "update:progress"
)

// startupUpdateDelay lets the application settle before it goes to the network.
const startupUpdateDelay = 5 * time.Second

// UpdateInfo is what the interface shows about the running build and any
// release waiting for it.
type UpdateInfo struct {
	// Version is the running build, for display.
	Version string `json:"version"`
	Detail  string `json:"detail"`
	// Installed distinguishes a copy put here by the installer from a portable
	// one, because only the former can be updated in place.
	Installed bool `json:"installed"`

	Status updater.Status `json:"status"`
}

// Version reports the running build. Used by the Info tab.
func (a *App) Version() UpdateInfo {
	return UpdateInfo{
		Version:   version.String(),
		Detail:    version.Detail(),
		Installed: updater.IsInstalled(),
	}
}

// CheckForUpdates asks GitHub whether a newer release exists.
//
// force is set when the user asks explicitly, which bypasses both the setting
// that turns checking off and any version they previously chose to skip.
func (a *App) CheckForUpdates(force bool) (UpdateInfo, error) {
	info := a.Version()

	current := a.settings.Get()
	if !force && !current.CheckForUpdates {
		info.Status = updater.Status{Message: "Update checking is turned off."}
		return info, nil
	}

	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	status, err := a.updates.Check(ctx)
	if err != nil {
		a.log.Warn("app: update check failed: %v", err)
		return info, fmt.Errorf("%v", err)
	}

	// Remember what the check found. InstallUpdate acts on this rather than on
	// anything the interface sends it, so a release can only be installed after
	// PlayerOne has fetched and compared it itself - and every path that offers
	// an update, the startup check and the Check now button alike, records it
	// here.
	if status.Available && status.Latest != nil {
		a.setLatestRelease(status.Latest)
	} else {
		a.setLatestRelease(nil)
	}

	// A skipped version stays skipped until something newer appears or the user
	// checks by hand.
	if !force && status.Available && status.Latest != nil &&
		status.Latest.Version == current.SkippedVersion {
		a.log.Info("app: update %s is available but was skipped", status.Latest.Version)
		status.Available = false
	}

	info.Status = status
	if status.Available && status.Latest != nil {
		a.log.Info("app: update available: %s (installer %s, checksums=%v)",
			status.Latest.Version, status.Latest.InstallerName, status.Latest.HasChecksums)
	} else {
		a.log.Info("app: update check: %s", status.Message)
	}
	return info, nil
}

// SkipUpdate stops PlayerOne offering this version again.
func (a *App) SkipUpdate(v string) {
	a.persist(func(s *settingsMutation) { s.SkippedVersion = v })
	a.log.Info("app: skipping version %s", v)
}

// SetCheckForUpdates turns automatic checking on or off.
func (a *App) SetCheckForUpdates(on bool) {
	a.persist(func(s *settingsMutation) { s.CheckForUpdates = on })
}

// OpenReleasePage opens a release in the browser.
//
// Only a URL from a release PlayerOne itself fetched is opened, never one
// supplied by the interface, so this cannot be turned into a way to launch an
// arbitrary link.
func (a *App) OpenReleasePage() error {
	release := a.latestRelease()
	if release == nil || release.URL == "" {
		return fmt.Errorf("there is no release page to open")
	}

	wailsruntime.BrowserOpenURL(a.ctx, release.URL)
	return nil
}

// InstallUpdate downloads the installer, verifies it and hands over to it.
//
// PlayerOne quits immediately afterwards: an installer cannot replace files
// that are still open. The installer restarts PlayerOne when it finishes.
func (a *App) InstallUpdate() error {
	release := a.latestRelease()
	if release == nil {
		return fmt.Errorf("check for updates first")
	}

	// A portable copy has no installer to update; running one would put a
	// second, installed copy somewhere else and leave this folder stale.
	if !updater.IsInstalled() {
		return fmt.Errorf(
			"this is a portable copy, so it cannot update itself. Download %s from the release page and replace this folder.",
			release.Version)
	}
	if release.InstallerURL == "" {
		return fmt.Errorf("release %s publishes no installer", release.Version)
	}

	// Left behind deliberately when the installer starts: it is running from
	// there. The next launch of PlayerOne sweeps it up.
	dir, err := updater.NewDownloadDir()
	if err != nil {
		return fmt.Errorf("%v", err)
	}

	a.log.Info("app: downloading update %s (%d bytes)", release.Version, release.InstallerSize)

	path, err := a.updates.Download(a.ctx, release, dir, func(done, total int64) {
		a.emit(eventUpdateProgress, map[string]any{"done": done, "total": total})
	})
	if err != nil {
		_ = os.RemoveAll(dir)
		a.log.Error("app: %v", err)
		return fmt.Errorf("%v", err)
	}

	if !release.HasChecksums {
		a.log.Warn("app: release %s published no checksums; the download could not be verified", release.Version)
	}

	a.log.Info("app: launching installer %s", filepath.Base(path))
	if err := updater.Launch(path); err != nil {
		_ = os.RemoveAll(dir)
		a.log.Error("app: %v", err)
		return fmt.Errorf("%v", err)
	}

	// The position of whatever is playing is saved before handing over, so an
	// update does not cost the viewer their place.
	a.saveResumePosition()

	// Quitting is deferred a moment so this call can return and the interface
	// can say what is happening.
	go func() {
		time.Sleep(750 * time.Millisecond)
		wailsruntime.Quit(a.ctx)
	}()

	return nil
}

func (a *App) latestRelease() *updater.Release {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.pendingUpdate
}

func (a *App) setLatestRelease(r *updater.Release) {
	a.mu.Lock()
	a.pendingUpdate = r
	a.mu.Unlock()
}

// runUpdateDownloadCleanup removes what earlier updates left in the temporary
// directory.
//
// Unconditional, unlike the update check: files already on disk should be
// cleaned up whether or not this copy is still willing to look for updates, and
// a development build can be started by an installer just as a release can.
func (a *App) runUpdateDownloadCleanup() {
	defer a.wg.Done()

	// The same delay as the update check, for a different reason: when this
	// copy was started from the installer's finish page, that installer is
	// still on its way out, and its own file cannot be deleted until it has
	// gone.
	select {
	case <-a.stopCh:
		return
	case <-time.After(startupUpdateDelay):
	}

	removed, err := updater.CleanDownloadDirs()
	if removed > 0 {
		a.log.Info("app: removed %d leftover update download(s)", removed)
	}
	if err != nil {
		// Not a failure worth telling anyone about: the folder is almost
		// certainly the installer that started this copy, and the next launch
		// will get it.
		a.log.Debug("app: %v", err)
	}
}

// runStartupUpdateCheck checks once, shortly after launch, if it is due.
func (a *App) runStartupUpdateCheck() {
	defer a.wg.Done()

	select {
	case <-a.stopCh:
		return
	case <-time.After(startupUpdateDelay):
	}

	current := a.settings.Get()
	if !current.CheckForUpdates {
		a.log.Debug("app: update checking is turned off")
		return
	}
	if !version.IsRelease() {
		a.log.Debug("app: development build; not checking for updates")
		return
	}

	// Every launch, not once a day: a release the viewer already knows about is
	// worth offering again, and one request to GitHub per start is nothing.
	info, err := a.CheckForUpdates(false)
	if err != nil {
		return // already logged; a failed check must not disturb the viewer
	}

	if info.Status.Available && info.Status.Latest != nil {
		a.emit(eventUpdateAvailable, info)
	}
}
