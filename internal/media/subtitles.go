package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// maxSubtitleBytes caps what will be read from a subtitle source.
//
// A three-hour transcript is on the order of a hundred kilobytes; anything past
// this is a malformed or hostile file, and reading it would only waste memory.
const maxSubtitleBytes = 32 << 20 // 32 MiB

// ErrImageBased is returned for subtitle tracks stored as pictures, which cannot
// become text without OCR.
var ErrImageBased = errors.New("media: this subtitle track is image-based, so it has no text to show")

// textSubtitleExtensions are the external subtitle files that can be read as
// text without conversion.
var textSubtitleExtensions = map[string]bool{
	".srt": true,
	".vtt": true,
}

// convertibleSubtitleExtensions need ffmpeg to become plain timed text.
var convertibleSubtitleExtensions = map[string]bool{
	".ass": true,
	".ssa": true,
	".sub": true,
	".sbv": true,
	".smi": true,
	".ttml": true,
	".dfxp": true,
}

// IsSubtitleFile reports whether a path looks like a subtitle file, which is how
// a dropped file is routed to "attach subtitle" rather than "open video".
func IsSubtitleFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return textSubtitleExtensions[ext] || convertibleSubtitleExtensions[ext]
}

// SubtitleExtensions lists every supported subtitle extension, for file dialogs
// and for documenting what a drop will accept.
func SubtitleExtensions() []string {
	out := make([]string, 0, len(textSubtitleExtensions)+len(convertibleSubtitleExtensions))
	for ext := range textSubtitleExtensions {
		out = append(out, ext)
	}
	for ext := range convertibleSubtitleExtensions {
		out = append(out, ext)
	}
	return out
}

// ExtractEmbedded pulls one subtitle track out of a media file as SubRip text.
//
// The track is identified by its ffmpeg stream index, which mpv reports as
// ff-index on each track-list entry. Using that index rather than counting
// subtitle tracks is what makes the extracted track match the one the user
// selected, even in files where stream order and track order differ.
//
// Output is streamed from ffmpeg's stdout, so no temporary file is created and
// there is nothing to clean up.
func ExtractEmbedded(ctx context.Context, ffmpegPath, mediaPath string, ffIndex int) (string, error) {
	if ffmpegPath == "" {
		return "", errors.New("media: ffmpeg is not available, so transcripts cannot be extracted from embedded subtitle tracks")
	}
	if ffIndex < 0 {
		return "", fmt.Errorf("media: this subtitle track has no stream index, so it cannot be extracted")
	}

	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-nostdin",
		"-v", "error",
		"-i", mediaPath,
		"-map", "0:"+strconv.Itoa(ffIndex),
		// SubRip is the target for every text format: the conversion preserves
		// timings exactly, and it leaves the parser with one shape to handle.
		"-f", "srt",
		"pipe:1",
	)
	configureProcAttr(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}

		// ffmpeg's own words are unhelpful to a viewer, so the common case is
		// translated into something actionable.
		if strings.Contains(strings.ToLower(detail), "encoder") || strings.Contains(strings.ToLower(detail), "subtitle encoding") {
			return "", ErrImageBased
		}
		return "", fmt.Errorf("media: extracting subtitle stream %d from %s: %s", ffIndex, filepath.Base(mediaPath), detail)
	}

	if stdout.Len() > maxSubtitleBytes {
		return "", fmt.Errorf("media: subtitle stream %d is unreasonably large (%d bytes)", ffIndex, stdout.Len())
	}

	text := decodeSubtitleBytes(stdout.Bytes())
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("media: subtitle stream %d contains no text", ffIndex)
	}
	return text, nil
}

// LoadExternal reads an external subtitle file as timed text, converting it with
// ffmpeg when the format is not already SubRip or WebVTT.
func LoadExternal(ctx context.Context, ffmpegPath, path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))

	if textSubtitleExtensions[ext] {
		raw, err := readCapped(path)
		if err != nil {
			return "", err
		}
		text := decodeSubtitleBytes(raw)
		if strings.TrimSpace(text) == "" {
			return "", fmt.Errorf("media: %s contains no subtitle text", filepath.Base(path))
		}
		return text, nil
	}

	if !convertibleSubtitleExtensions[ext] {
		return "", fmt.Errorf("media: %s is not a subtitle format PlayerOne can read", filepath.Base(path))
	}

	// ASS/SSA and the rest go through ffmpeg. Converting to SubRip drops styling
	// but keeps every timing, which is exactly the trade a transcript wants.
	if ffmpegPath == "" {
		return "", fmt.Errorf("media: reading %s needs ffmpeg, which was not found", filepath.Base(path))
	}

	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-nostdin",
		"-v", "error",
		"-i", path,
		"-f", "srt",
		"pipe:1",
	)
	configureProcAttr(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("media: converting %s: %s", filepath.Base(path), detail)
	}

	text := decodeSubtitleBytes(stdout.Bytes())
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("media: %s contains no subtitle text", filepath.Base(path))
	}
	return text, nil
}

func readCapped(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("media: opening %s: %w", filepath.Base(path), err)
	}
	defer f.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(io.LimitReader(f, maxSubtitleBytes)); err != nil {
		return nil, fmt.Errorf("media: reading %s: %w", filepath.Base(path), err)
	}
	return buf.Bytes(), nil
}

// decodeSubtitleBytes turns raw subtitle bytes into a Go string.
//
// Downloaded subtitle files are usually UTF-8, but files produced by older
// Windows tools are frequently in a single-byte codepage. Rather than guess an
// encoding wrongly, invalid bytes are mapped through Latin-1, which never fails
// and leaves ASCII - the overwhelming majority of any subtitle file - correct.
// UTF-16 is detected from its byte order mark, since that is unambiguous.
func decodeSubtitleBytes(raw []byte) string {
	if text, ok := decodeUTF16(raw); ok {
		return text
	}

	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM

	if utf8.Valid(raw) {
		return string(raw)
	}

	var b strings.Builder
	b.Grow(len(raw))
	for _, c := range raw {
		b.WriteRune(rune(c))
	}
	return b.String()
}

func decodeUTF16(raw []byte) (string, bool) {
	if len(raw) < 2 {
		return "", false
	}

	var littleEndian bool
	switch {
	case raw[0] == 0xFF && raw[1] == 0xFE:
		littleEndian = true
	case raw[0] == 0xFE && raw[1] == 0xFF:
		littleEndian = false
	default:
		return "", false
	}

	body := raw[2:]
	units := make([]uint16, 0, len(body)/2)
	for i := 0; i+1 < len(body); i += 2 {
		if littleEndian {
			units = append(units, uint16(body[i])|uint16(body[i+1])<<8)
		} else {
			units = append(units, uint16(body[i])<<8|uint16(body[i+1]))
		}
	}

	return string(utf16.Decode(units)), true
}
