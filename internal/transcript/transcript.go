// Package transcript turns timed subtitle text into the entries the transcript
// panel displays.
//
// It handles SubRip (.srt) and WebVTT (.vtt) only. Every other format reaches
// this package already converted to SubRip by ffmpeg (see internal/media), which
// keeps the parsing surface small and the timing exact.
package transcript

import (
	"fmt"
	"strings"
	"unicode"
)

// Entry is one timed line of transcript.
type Entry struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// Format identifies a subtitle text format.
type Format string

const (
	FormatSRT     Format = "srt"
	FormatWebVTT  Format = "vtt"
	FormatUnknown Format = ""
)

// Detect sniffs the format of subtitle text.
//
// WebVTT is required by its own specification to begin with the "WEBVTT" magic,
// so the check is exact rather than heuristic. Anything else that parses is
// treated as SubRip, which is what ffmpeg emits for us.
func Detect(text string) Format {
	trimmed := strings.TrimLeft(text, "\ufeff \t\r\n")

	if strings.HasPrefix(trimmed, "WEBVTT") {
		return FormatWebVTT
	}
	if trimmed == "" {
		return FormatUnknown
	}
	return FormatSRT
}

// Parse converts subtitle text into transcript entries, choosing the parser by
// sniffing the content.
//
// Parsing is deliberately lenient. A tutorial video's subtitles are frequently
// machine-generated and slightly malformed, and dropping one bad cue is always
// better than refusing to show a transcript at all. An error is returned only
// when nothing at all could be parsed.
func Parse(text string) ([]Entry, error) {
	switch Detect(text) {
	case FormatWebVTT:
		return ParseVTT(text)
	case FormatSRT:
		return ParseSRT(text)
	default:
		return nil, fmt.Errorf("transcript: subtitle data is empty")
	}
}

// normaliseNewlines makes the line-splitting logic independent of whether the
// file came from Windows, Unix or a classic Mac tool, and strips a BOM.
func normaliseNewlines(text string) string {
	text = strings.TrimPrefix(text, "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}

// joinCueLines collapses a multi-line cue into the single line the transcript
// panel shows.
//
// Subtitles are wrapped for on-screen display, which is meaningless in a
// scrolling transcript, so the wrap is undone. Non-breaking spaces are folded to
// ordinary spaces because the real-world fixture is full of them and they would
// otherwise survive whitespace collapsing and look like stray gaps.
func joinCueLines(lines []string) string {
	var parts []string
	for _, line := range lines {
		if s := collapseSpaces(line); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

func collapseSpaces(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	space := false
	for _, r := range s {
		// U+00A0 and friends are spaces for our purposes, but unicode.IsSpace
		// already reports true for them, so one test covers both.
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}
