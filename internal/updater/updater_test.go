package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"playerone/internal/version"
)

// fakeGitHub serves a release payload plus its assets.
type fakeGitHub struct {
	server *httptest.Server

	installer     []byte
	installerName string
	withChecksums bool
	corruptSums   bool
}

func newFakeGitHub(t *testing.T, tag string, installer []byte, withChecksums bool) *fakeGitHub {
	t.Helper()

	f := &fakeGitHub{
		installer:     installer,
		installerName: "PlayerOne-" + strings.TrimPrefix(tag, "v") + "-windows-amd64-installer.exe",
		withChecksums: withChecksums,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/repos/"+Owner+"/"+Repo+"/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		assets := fmt.Sprintf(
			`{"name":%q,"size":%d,"browser_download_url":"%s/download/%s"}`,
			f.installerName, len(f.installer), f.server.URL, f.installerName)

		if f.withChecksums {
			assets += fmt.Sprintf(
				`,{"name":%q,"size":1,"browser_download_url":"%s/download/%s"}`,
				checksumAsset, f.server.URL, checksumAsset)
		}

		fmt.Fprintf(w, `{
			"tag_name": %q,
			"name": "PlayerOne %s",
			"body": "Release notes here.",
			"html_url": "https://example.invalid/release",
			"draft": false,
			"prerelease": false,
			"published_at": "2026-09-10T00:00:00Z",
			"assets": [%s]
		}`, tag, tag, assets)
	})

	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)

		if name == checksumAsset {
			sum := sha256.Sum256(f.installer)
			digest := hex.EncodeToString(sum[:])
			if f.corruptSums {
				digest = strings.Repeat("0", 64)
			}
			fmt.Fprintf(w, "%s  %s\n", digest, f.installerName)
			return
		}

		if name == f.installerName {
			_, _ = w.Write(f.installer)
			return
		}

		http.NotFound(w, r)
	})

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeGitHub) client() *Client {
	return NewWithAPI(f.server.URL, f.server.Client())
}

// asRelease pins the build's version for the duration of a test.
func asRelease(t *testing.T, v string) {
	t.Helper()
	original := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = original })
}

func TestCheckFindsANewerRelease(t *testing.T) {
	asRelease(t, "1.0.0")
	fake := newFakeGitHub(t, "v1.1.0", []byte("installer bytes"), true)

	status, err := fake.client().Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	if !status.Available {
		t.Fatalf("expected an update to be available: %+v", status)
	}
	if status.Latest == nil || status.Latest.Version != "1.1.0" {
		t.Fatalf("Latest = %+v", status.Latest)
	}
	if status.Latest.InstallerURL == "" {
		t.Error("the installer asset was not found")
	}
	if !status.Latest.HasChecksums {
		t.Error("HasChecksums = false, but the release publishes them")
	}
	if !strings.Contains(status.Message, "1.1.0") {
		t.Errorf("Message = %q, want it to name the new version", status.Message)
	}
}

func TestCheckOnTheLatestVersion(t *testing.T) {
	asRelease(t, "1.1.0")
	fake := newFakeGitHub(t, "v1.1.0", []byte("installer"), true)

	status, err := fake.client().Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if status.Available {
		t.Error("Available = true when already on the latest version")
	}
	if !strings.Contains(status.Message, "latest") {
		t.Errorf("Message = %q", status.Message)
	}
}

func TestCheckDoesNotOfferAnOlderRelease(t *testing.T) {
	asRelease(t, "2.0.0")
	fake := newFakeGitHub(t, "v1.0.0", []byte("installer"), true)

	status, err := fake.client().Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if status.Available {
		t.Error("Available = true for an older release")
	}
}

// A working tree must never be offered a "update" to a published release.
func TestCheckSkipsDevelopmentBuilds(t *testing.T) {
	asRelease(t, "dev")
	fake := newFakeGitHub(t, "v9.9.9", []byte("installer"), true)

	status, err := fake.client().Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if status.Available {
		t.Error("a development build should never be offered an update")
	}
	if !strings.Contains(status.Message, "development build") {
		t.Errorf("Message = %q", status.Message)
	}
}

func TestDownloadVerifiesTheChecksum(t *testing.T) {
	asRelease(t, "1.0.0")
	payload := []byte("a convincing installer")
	fake := newFakeGitHub(t, "v1.1.0", payload, true)

	client := fake.client()
	status, err := client.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	dir := t.TempDir()
	path, err := client.Download(context.Background(), status.Latest, dir, nil)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the download: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("downloaded %q, want %q", got, payload)
	}
}

// The security-critical case: bytes that do not match the published checksum
// must be refused and deleted, never handed to the installer.
func TestDownloadRefusesAMismatchedChecksum(t *testing.T) {
	asRelease(t, "1.0.0")
	fake := newFakeGitHub(t, "v1.1.0", []byte("tampered installer"), true)
	fake.corruptSums = true

	client := fake.client()
	status, err := client.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	dir := t.TempDir()
	path, err := client.Download(context.Background(), status.Latest, dir, nil)
	if err == nil {
		t.Fatal("expected the download to be refused")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("error = %v, want it to mention the checksum", err)
	}
	if path != "" {
		t.Errorf("a path was returned for a refused download: %q", path)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the download directory: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("the rejected download was left on disk: %v", entries)
	}
}

// A release without checksums still installs; the weaker guarantee is the
// interface's to explain, not a reason to fail.
func TestDownloadWithoutChecksums(t *testing.T) {
	asRelease(t, "1.0.0")
	fake := newFakeGitHub(t, "v1.1.0", []byte("installer"), false)

	client := fake.client()
	status, err := client.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if status.Latest.HasChecksums {
		t.Error("HasChecksums = true when the release publishes none")
	}

	if _, err := client.Download(context.Background(), status.Latest, t.TempDir(), nil); err != nil {
		t.Fatalf("Download: %v", err)
	}
}

func TestDownloadReportsProgress(t *testing.T) {
	asRelease(t, "1.0.0")
	fake := newFakeGitHub(t, "v1.1.0", []byte(strings.Repeat("x", 1024)), true)

	client := fake.client()
	status, _ := client.Check(context.Background())

	var calls int
	var lastDone int64
	_, err := client.Download(context.Background(), status.Latest, t.TempDir(), func(done, _ int64) {
		calls++
		lastDone = done
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if calls == 0 {
		t.Error("progress was never reported")
	}
	if lastDone != 1024 {
		t.Errorf("final progress = %d, want 1024", lastDone)
	}
}

func TestDownloadRejectsATruncatedFile(t *testing.T) {
	asRelease(t, "1.0.0")
	fake := newFakeGitHub(t, "v1.1.0", []byte("short"), false)

	client := fake.client()
	status, _ := client.Check(context.Background())

	// Claim a larger asset than the server will actually serve.
	status.Latest.InstallerSize = 5000

	dir := t.TempDir()
	if _, err := client.Download(context.Background(), status.Latest, dir, nil); err == nil {
		t.Fatal("expected a truncated download to be rejected")
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("the truncated download was left behind: %v", entries)
	}
}

func TestCheckHandlesNoReleases(t *testing.T) {
	asRelease(t, "1.0.0")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	_, err := NewWithAPI(server.URL, server.Client()).Check(context.Background())
	if err == nil {
		t.Fatal("expected an error when there are no releases")
	}
	if !strings.Contains(err.Error(), "no releases") {
		t.Errorf("error = %v, want it to say there are no releases", err)
	}
}

func TestCheckHandlesRateLimiting(t *testing.T) {
	asRelease(t, "1.0.0")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	_, err := NewWithAPI(server.URL, server.Client()).Check(context.Background())
	if err == nil {
		t.Fatal("expected an error when rate-limited")
	}
	if !strings.Contains(err.Error(), "rate-limiting") {
		t.Errorf("error = %v, want it to explain the rate limit", err)
	}
}

func TestCheckIgnoresDrafts(t *testing.T) {
	asRelease(t, "1.0.0")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v2.0.0","draft":true,"assets":[]}`)
	}))
	t.Cleanup(server.Close)

	if _, err := NewWithAPI(server.URL, server.Client()).Check(context.Background()); err == nil {
		t.Fatal("expected a draft release to be rejected")
	}
}

// The asset name arrives over the network, so it must never be able to steer
// the download outside the directory it was given.
func TestSafeFileName(t *testing.T) {
	const fallback = "PlayerOne-installer.exe"

	cases := []struct {
		in   string
		want string
	}{
		{"PlayerOne-1.0.0-windows-amd64-installer.exe", "PlayerOne-1.0.0-windows-amd64-installer.exe"},
		{"", fallback},
		{"   ", fallback},
		{"..", fallback},
		{`..\..\Windows\System32\evil.exe`, fallback},
		{"../../etc/passwd", fallback},
		{"notanexe.zip", fallback},
		{"C:\\Windows\\evil.exe", fallback},
		{"has:colon.exe", fallback},
	}

	for _, tc := range cases {
		got := safeFileName(tc.in, fallback)
		if got != tc.want {
			t.Errorf("safeFileName(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if strings.ContainsAny(got, `/\`) {
			t.Errorf("safeFileName(%q) = %q, which contains a path separator", tc.in, got)
		}
	}
}
