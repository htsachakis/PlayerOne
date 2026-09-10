package playlist

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"playerone/internal/appdir"
)

// Extension is the file extension saved playlists use.
//
// M3U8 rather than a format of our own: a saved playlist then opens in VLC,
// mpv, foobar2000 and everything else, and can be copied to another machine or
// handed to someone. A private JSON blob would work only here.
const Extension = ".m3u8"

// maxPlaylistBytes caps what will be read from a playlist file. A thousand
// entries is well under a megabyte; beyond this it is not a playlist.
const maxPlaylistBytes = 8 << 20

// WriteM3U saves items as an M3U8 playlist.
//
// Paths are written absolute. A saved playlist commonly lives in the
// application's own folder while the media sits on another drive entirely, and
// a relative path would only be correct by accident.
func WriteM3U(path string, items []Item) error {
	var b strings.Builder

	b.WriteString("#EXTM3U\n")
	b.WriteString("# Saved by PlayerOne\n")

	for _, item := range items {
		if strings.TrimSpace(item.Path) == "" {
			continue
		}

		// #EXTINF carries the duration and title, so another player can show
		// them without opening every file.
		seconds := -1
		if item.Duration > 0 {
			seconds = int(item.Duration + 0.5)
		}

		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = item.Filename
		}
		// A newline in a title would forge a new entry.
		title = strings.NewReplacer("\r", " ", "\n", " ").Replace(title)

		fmt.Fprintf(&b, "#EXTINF:%d,%s\n", seconds, title)

		abs := item.Path
		if resolved, err := filepath.Abs(abs); err == nil {
			abs = resolved
		}
		b.WriteString(abs)
		b.WriteByte('\n')
	}

	if err := appdir.WriteFileAtomic(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("playlist: saving %s: %w", filepath.Base(path), err)
	}
	return nil
}

// ReadM3U loads an M3U or M3U8 playlist.
//
// Relative entries are resolved against the playlist file's own directory,
// which is what every other player does and what makes a playlist saved beside
// its media portable.
func ReadM3U(path string) ([]Item, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("playlist: opening %s: %w", filepath.Base(path), err)
	}
	defer file.Close()

	items, err := parseM3U(io.LimitReader(file, maxPlaylistBytes), filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("playlist: %s contains no entries", filepath.Base(path))
	}
	return items, nil
}

// parseM3U does the work, separated from the file handling so it can be tested
// without touching disk.
func parseM3U(r io.Reader, baseDir string) ([]Item, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxPlaylistBytes)

	var (
		items       []Item
		pendingName string
		pendingSecs float64
		seen        = map[string]bool{}
		first       = true
	)

	for scanner.Scan() {
		line := scanner.Text()

		if first {
			// A UTF-8 byte order mark would otherwise become part of the first
			// path, which is a common way for playlists written by other tools
			// to fail to load.
			line = strings.TrimPrefix(line, "\ufeff")
			first = false
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "#") {
			if strings.HasPrefix(line, "#EXTINF:") {
				pendingSecs, pendingName = parseExtInf(line)
			}
			// Every other directive, including #EXTM3U itself, carries nothing
			// this player needs.
			continue
		}

		// Anything that is not a local file is skipped rather than queued: a
		// remote URL would be handed to mpv, which is a different feature and
		// one with its own consequences.
		if isRemote(line) {
			pendingName, pendingSecs = "", 0
			continue
		}

		path := line
		if !filepath.IsAbs(path) && baseDir != "" {
			path = filepath.Join(baseDir, path)
		}
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}

		if seen[key(path)] {
			pendingName, pendingSecs = "", 0
			continue
		}
		seen[key(path)] = true

		item := Item{
			Path:     path,
			Filename: filepath.Base(path),
			Title:    pendingName,
			Duration: pendingSecs,
		}
		items = append(items, item)

		pendingName, pendingSecs = "", 0
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("playlist: reading the playlist: %w", err)
	}
	return items, nil
}

// parseExtInf reads "#EXTINF:<seconds>,<title>".
func parseExtInf(line string) (seconds float64, title string) {
	rest := strings.TrimPrefix(line, "#EXTINF:")

	value, name, found := strings.Cut(rest, ",")
	if found {
		title = strings.TrimSpace(name)
	}

	// The duration field may carry trailing attributes in extended variants;
	// only the leading number is of interest, and -1 means "unknown".
	value = strings.TrimSpace(value)
	if space := strings.IndexAny(value, " \t"); space >= 0 {
		value = value[:space]
	}
	if n, err := strconv.ParseFloat(value, 64); err == nil && n > 0 {
		seconds = n
	}

	if !utf8.ValidString(title) {
		title = ""
	}
	return seconds, title
}

// isRemote reports whether a line names something other than a local file.
func isRemote(line string) bool {
	for _, scheme := range []string{"http://", "https://", "rtsp://", "rtmp://", "udp://", "ftp://", "smb://"} {
		if strings.HasPrefix(strings.ToLower(line), scheme) {
			return true
		}
	}
	return false
}
