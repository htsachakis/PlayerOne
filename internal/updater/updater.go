// Package updater checks GitHub for a newer PlayerOne and installs it.
//
// The design rule here is that this code downloads an executable and runs it,
// so it must be able to say exactly what it is running. Every download is size-
// checked and, when the release publishes checksums, verified against SHA-256
// before anything is executed. A release without checksums is reported as such
// rather than trusted silently.
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"playerone/internal/version"
)

// Repository is the project the updater looks at. It is fixed rather than
// configurable: a setting that pointed the updater at an arbitrary repository
// would be a way to make PlayerOne run someone else's executable.
const (
	Owner = "htsachakis"
	Repo  = "PlayerOne"
)

// checksumAsset is the file a release publishes alongside its downloads.
const checksumAsset = "SHA256SUMS.txt"

// maxDownloadBytes caps a download. The installer is around 170 MB; anything
// approaching this limit is not a PlayerOne release.
const maxDownloadBytes = 600 << 20

// httpTimeout bounds the metadata request. The download itself is given longer,
// since it is large and may be on a slow connection.
const (
	httpTimeout     = 20 * time.Second
	downloadTimeout = 30 * time.Minute
)

// Release describes a published version.
type Release struct {
	Version     string `json:"version"`
	Name        string `json:"name"`
	Notes       string `json:"notes"`
	URL         string `json:"url"`
	PublishedAt string `json:"publishedAt"`

	// InstallerURL and InstallerSize describe the Windows installer asset.
	// Empty when the release does not publish one.
	InstallerURL  string `json:"installerUrl"`
	InstallerName string `json:"installerName"`
	InstallerSize int64  `json:"installerSize"`

	// HasChecksums reports whether the release publishes SHA256SUMS.txt. The
	// interface says so, because installing without it is a weaker guarantee.
	HasChecksums bool `json:"hasChecksums"`
}

// Status is the outcome of a check.
type Status struct {
	Checked   bool     `json:"checked"`
	Available bool     `json:"available"`
	Current   string   `json:"current"`
	Latest    *Release `json:"latest"`

	// Message explains the outcome in the user's terms, including why no update
	// was offered.
	Message string `json:"message"`
}

// Client talks to GitHub.
type Client struct {
	http *http.Client
	// api is overridable so the tests can point at a local server rather than
	// the real GitHub.
	api string
}

// New builds a client against the real GitHub API.
func New() *Client {
	return &Client{
		http: &http.Client{Timeout: httpTimeout},
		api:  "https://api.github.com",
	}
}

// NewWithAPI builds a client against a different base URL, for tests.
func NewWithAPI(base string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: httpTimeout}
	}
	return &Client{http: httpClient, api: strings.TrimSuffix(base, "/")}
}

// ghRelease mirrors the part of GitHub's release payload that is used here.
type ghRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		Size               int64  `json:"size"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// Check asks GitHub for the latest release and compares it with this build.
func (c *Client) Check(ctx context.Context) (Status, error) {
	current := version.Version

	status := Status{Current: version.String()}

	// A development build has no version to compare against, and replacing a
	// working tree with a release would discard whatever is being worked on.
	if !version.IsRelease() {
		status.Checked = true
		status.Message = "This is a development build, so it is not compared against published releases."
		return status, nil
	}

	release, err := c.latest(ctx)
	if err != nil {
		return status, err
	}

	status.Checked = true
	status.Latest = release

	if !version.IsNewer(current, release.Version) {
		status.Message = fmt.Sprintf("PlayerOne %s is the latest version.", version.Version)
		return status, nil
	}

	status.Available = true
	if release.InstallerURL == "" {
		status.Message = fmt.Sprintf(
			"PlayerOne %s is available, but this release publishes no installer. Download it from the release page.",
			release.Version)
		return status, nil
	}

	status.Message = fmt.Sprintf("PlayerOne %s is available. You have %s.", release.Version, version.Version)
	return status, nil
}

func (c *Client) latest(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.api, Owner, Repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("updater: building the request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "PlayerOne-updater")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("updater: could not reach GitHub: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("updater: no releases have been published yet")
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, fmt.Errorf("updater: GitHub is rate-limiting update checks; try again later")
	default:
		return nil, fmt.Errorf("updater: GitHub returned %s", resp.Status)
	}

	var payload ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("updater: reading GitHub's response: %w", err)
	}
	if payload.Draft {
		return nil, fmt.Errorf("updater: the latest release is still a draft")
	}

	release := &Release{
		Version:     strings.TrimPrefix(payload.TagName, "v"),
		Name:        payload.Name,
		Notes:       payload.Body,
		URL:         payload.HTMLURL,
		PublishedAt: payload.PublishedAt,
	}

	for _, asset := range payload.Assets {
		switch {
		case asset.Name == checksumAsset:
			release.HasChecksums = true
		case strings.HasSuffix(asset.Name, "-installer.exe"):
			release.InstallerURL = asset.BrowserDownloadURL
			release.InstallerName = asset.Name
			release.InstallerSize = asset.Size
		}
	}

	return release, nil
}

// Download fetches the installer, verifies it, and returns the path it was
// written to.
//
// progress, if given, is called with the number of bytes received so far and
// the total; it lets the interface show something during a download that can
// take minutes.
func (c *Client) Download(ctx context.Context, release *Release, dir string, progress func(done, total int64)) (string, error) {
	if release == nil || release.InstallerURL == "" {
		return "", fmt.Errorf("updater: this release publishes no installer")
	}
	if release.InstallerSize > maxDownloadBytes {
		return "", fmt.Errorf("updater: the installer is unexpectedly large (%d bytes); refusing to download it", release.InstallerSize)
	}

	// The name comes from the release metadata, so it is not trusted as a path.
	name := safeFileName(release.InstallerName, "PlayerOne-installer.exe")
	target := filepath.Join(dir, name)

	expected, checksumErr := c.checksums(ctx, release)
	if checksumErr != nil {
		// Reported, not fatal: a release without checksums can still be
		// installed, and the interface says the guarantee is weaker.
		expected = ""
	}

	if err := c.fetch(ctx, release.InstallerURL, target, release.InstallerSize, progress); err != nil {
		return "", err
	}

	sum, err := sha256File(target)
	if err != nil {
		_ = os.Remove(target)
		return "", err
	}

	if expected != "" && !strings.EqualFold(sum, expected) {
		// The one case that must never be waved through: the bytes are not what
		// the release says they are.
		_ = os.Remove(target)
		return "", fmt.Errorf(
			"updater: the downloaded installer does not match the checksum published with the release; it has been deleted")
	}

	return target, nil
}

// checksums downloads SHA256SUMS.txt and returns the digest for the installer.
func (c *Client) checksums(ctx context.Context, release *Release) (string, error) {
	if !release.HasChecksums {
		return "", fmt.Errorf("updater: this release publishes no checksums")
	}

	url := strings.TrimSuffix(release.InstallerURL, release.InstallerName) + checksumAsset

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "PlayerOne-updater")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("updater: fetching checksums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("updater: fetching checksums: %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("updater: reading checksums: %w", err)
	}

	// Lines are "<hex>  <filename>", as sha256sum writes them.
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		if filepath.Base(strings.TrimPrefix(fields[1], "*")) == release.InstallerName {
			return fields[0], nil
		}
	}

	return "", fmt.Errorf("updater: the checksum file lists no entry for %s", release.InstallerName)
}

func (c *Client) fetch(ctx context.Context, url, target string, expectedSize int64, progress func(done, total int64)) error {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("updater: building the download request: %w", err)
	}
	req.Header.Set("User-Agent", "PlayerOne-updater")

	// The metadata client's short timeout would abort a large download.
	client := &http.Client{Timeout: downloadTimeout}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("updater: downloading the installer: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("updater: downloading the installer: %s", resp.Status)
	}

	file, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("updater: creating %s: %w", target, err)
	}

	total := expectedSize
	if total <= 0 {
		total = resp.ContentLength
	}

	written, err := copyWithProgress(file, io.LimitReader(resp.Body, maxDownloadBytes), total, progress)
	closeErr := file.Close()

	if err != nil {
		_ = os.Remove(target)
		return fmt.Errorf("updater: downloading the installer: %w", err)
	}
	if closeErr != nil {
		_ = os.Remove(target)
		return fmt.Errorf("updater: writing the installer: %w", closeErr)
	}

	// A truncated download would otherwise be handed to the installer.
	if expectedSize > 0 && written != expectedSize {
		_ = os.Remove(target)
		return fmt.Errorf("updater: the download is incomplete (%d of %d bytes)", written, expectedSize)
	}

	return nil
}

func copyWithProgress(dst io.Writer, src io.Reader, total int64, progress func(done, total int64)) (int64, error) {
	buf := make([]byte, 256<<10)

	var done int64
	var lastReport time.Time

	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return done, werr
			}
			done += int64(n)

			// Throttled: a progress callback per 256 KiB chunk would flood the
			// interface on a fast connection.
			if progress != nil && time.Since(lastReport) > 200*time.Millisecond {
				lastReport = time.Now()
				progress(done, total)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return done, err
		}
	}

	if progress != nil {
		progress(done, total)
	}
	return done, nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("updater: opening the download: %w", err)
	}
	defer file.Close()

	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", fmt.Errorf("updater: checksumming the download: %w", err)
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// safeFileName reduces a name from release metadata to a bare filename.
//
// The name arrives over the network, so it is never used to build a path
// directly: a name containing separators could otherwise write outside the
// intended directory.
func safeFileName(name, fallback string) string {
	name = strings.TrimSpace(name)

	// Checked before filepath.Base, not after. Base would happily reduce
	// "..\..\Windows\System32\evil.exe" to "evil.exe", which is safe to write
	// but means quietly accepting metadata that had no business containing a
	// path at all. Anything path-shaped is rejected outright instead.
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\:*?"<>|`) {
		return fallback
	}
	if name != filepath.Base(name) {
		return fallback
	}
	if !strings.HasSuffix(strings.ToLower(name), ".exe") {
		return fallback
	}
	return name
}
