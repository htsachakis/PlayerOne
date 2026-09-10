package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"playerone/internal/logging"
	"playerone/internal/settings"
	"playerone/internal/updater"
	"playerone/internal/version"
)

// newTestApp builds the smallest App that can answer an update check.
func newTestApp(t *testing.T, api string, client *http.Client) *App {
	t.Helper()

	store, err := settings.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("settings.NewStore: %v", err)
	}

	app := NewApp()
	app.ctx = context.Background()
	app.log = logging.New(logging.LevelError)
	app.settings = store
	app.updates = updater.NewWithAPI(api, client)
	return app
}

// releaseServer stands in for GitHub, publishing one release.
func releaseServer(t *testing.T, tag string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases/latest") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": tag,
			"name":     "PlayerOne " + strings.TrimPrefix(tag, "v"),
			"html_url": "https://example.invalid/releases/" + tag,
			"assets": []map[string]any{
				{
					"name":                 "PlayerOne-" + strings.TrimPrefix(tag, "v") + "-windows-amd64-installer.exe",
					"size":                 182950147,
					"browser_download_url": "https://example.invalid/installer.exe",
				},
				{
					"name":                 "SHA256SUMS.txt",
					"size":                 221,
					"browser_download_url": "https://example.invalid/SHA256SUMS.txt",
				},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// asRelease makes the running build look like a published one, since the
// updater deliberately refuses to compare a development build.
func asRelease(t *testing.T, v string) {
	t.Helper()
	previous := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = previous })
}

// The bug this covers: the check found 1.0.2 and the interface offered it, but
// only the startup check recorded the release, so pressing Update now after a
// manual check answered "check for updates first".
func TestCheckForUpdatesRecordsTheReleaseForInstalling(t *testing.T) {
	asRelease(t, "1.0.1")
	srv := releaseServer(t, "v1.0.2")
	app := newTestApp(t, srv.URL, srv.Client())

	info, err := app.CheckForUpdates(true)
	if err != nil {
		t.Fatalf("CheckForUpdates: %v", err)
	}
	if !info.Status.Available {
		t.Fatalf("1.0.2 should be offered to 1.0.1: %+v", info.Status)
	}

	release := app.latestRelease()
	if release == nil {
		t.Fatal("the check offered an update but recorded nothing for InstallUpdate to install")
	}
	if release.Version != "1.0.2" {
		t.Errorf("recorded version = %q, want 1.0.2", release.Version)
	}
	if release.InstallerURL == "" {
		t.Error("the recorded release carries no installer")
	}
}

// A skipped version is still recorded: the bar is hidden, but a manual check
// from the settings panel must leave something installable behind.
func TestASkippedVersionIsStillRecorded(t *testing.T) {
	asRelease(t, "1.0.1")
	srv := releaseServer(t, "v1.0.2")
	app := newTestApp(t, srv.URL, srv.Client())

	app.SkipUpdate("1.0.2")

	info, err := app.CheckForUpdates(false)
	if err != nil {
		t.Fatalf("CheckForUpdates: %v", err)
	}
	if info.Status.Available {
		t.Error("a skipped version should not be offered by an automatic check")
	}
	if app.latestRelease() == nil {
		t.Error("the release should still be recorded so an explicit install can use it")
	}
}

// Nothing newer must leave nothing installable behind, so a stale release from
// an earlier check cannot be installed over a build that has caught up.
func TestAnUpToDateCheckClearsTheRecordedRelease(t *testing.T) {
	asRelease(t, "1.0.2")
	srv := releaseServer(t, "v1.0.2")
	app := newTestApp(t, srv.URL, srv.Client())

	app.setLatestRelease(&updater.Release{Version: "1.0.2"})

	if _, err := app.CheckForUpdates(true); err != nil {
		t.Fatalf("CheckForUpdates: %v", err)
	}
	if app.latestRelease() != nil {
		t.Error("an up-to-date check should leave nothing to install")
	}
}

// InstallUpdate must refuse rather than act on nothing.
func TestInstallUpdateWithoutACheckRefuses(t *testing.T) {
	app := newTestApp(t, "https://example.invalid", nil)

	if err := app.InstallUpdate(); err == nil {
		t.Error("expected an error when no check has recorded a release")
	}
}
