// Package notes reads and writes the plain-text .notes files that hold a
// video's timestamped notes.
//
// The format is deliberately something a person can type in Notepad: a
// timestamp starts a note, anything else continues the previous one. Parsing is
// lenient because the file is expected to be hand-edited, and losing somebody's
// notes to a stray character would be far worse than accepting a line we did
// not quite understand.
package notes

import (
	"bufio"
	"io"
	"sort"
	"strconv"
	"strings"

	"playerone/internal/timefmt"
)

// Extension is the suffix of a notes file. It follows the video's own
// extension, so lesson.mp4 and lesson.mkv keep separate notes.
const Extension = ".notes"

// continuationIndent aligns wrapped lines under the text column, past the
// "HH:MM:SS " a note begins with.
const continuationIndent = "         "

// Note is one timestamped note. Empty text is a bookmark, which is the whole of
// the distinction between the two.
type Note struct {
	Time    float64 `json:"time"`
	Text    string  `json:"text"`
	Starred bool    `json:"starred"`
}

// Parse reads notes from a .notes file.
//
// It never fails on content: an unreadable line becomes note text rather than an
// error. Only a failure to read the stream itself is returned.
func Parse(r io.Reader) ([]Note, error) {
	var (
		out     []Note
		current *Note
		pending []string
	)

	flush := func() {
		if current == nil {
			return
		}
		current.Text = strings.TrimRight(strings.Join(pending, "\n"), "\n")
		out = append(out, *current)
		current, pending = nil, nil
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)

		if seconds, rest, ok := parseTimestamp(trimmed); ok {
			flush()
			text, starred := parseStar(rest)
			current = &Note{Time: seconds, Starred: starred}
			pending = []string{text}
			continue
		}

		// Everything before the first timestamp is a header, not a note.
		if current == nil {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		pending = append(pending, trimmed)
	}
	flush()

	if err := scanner.Err(); err != nil {
		return out, err
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out, nil
}

// parseTimestamp splits a leading HH:MM:SS, H:MM:SS or MM:SS off a line.
//
// The remainder is returned with its leading whitespace removed. A line whose
// leading run of digits is not a timestamp - "42 is the answer" - is reported as
// not a timestamp at all, so it becomes note text.
func parseTimestamp(line string) (float64, string, bool) {
	i := 0
	for i < len(line) && (line[i] == ':' || line[i] == '.' || (line[i] >= '0' && line[i] <= '9')) {
		i++
	}
	if i == 0 {
		return 0, "", false
	}

	parts := strings.Split(line[:i], ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, "", false
	}

	var seconds float64
	for _, part := range parts {
		value, err := strconv.ParseFloat(part, 64)
		if err != nil || value < 0 {
			return 0, "", false
		}
		seconds = seconds*60 + value
	}

	return seconds, strings.TrimLeft(line[i:], " \t"), true
}

// parseStar peels the star marker off a note's first line.
//
// A note whose text genuinely begins with an asterisk is written escaped, and
// that escape is undone here. It is the only escape in the format.
func parseStar(rest string) (string, bool) {
	switch {
	case rest == "*":
		return "", true
	case strings.HasPrefix(rest, "* "):
		return strings.TrimLeft(rest[2:], " \t"), true
	case strings.HasPrefix(rest, `\*`):
		return rest[1:], false
	}
	return rest, false
}

// Format renders notes as the text of a .notes file, sorted by timestamp.
//
// CRLF line endings, because the first thing anyone does with one of these files
// on Windows is open it in an editor.
func Format(notes []Note, videoName string) []byte {
	sorted := append([]Note(nil), notes...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Time < sorted[j].Time })

	var b strings.Builder
	b.WriteString("# PlayerOne notes")
	if videoName != "" {
		b.WriteString(" - ")
		b.WriteString(videoName)
	}
	b.WriteString("\r\n\r\n")

	for _, note := range sorted {
		lines := strings.Split(note.Text, "\n")

		first := lines[0]
		if strings.HasPrefix(first, "*") {
			first = `\` + first
		}

		b.WriteString(timefmt.FormatClock(note.Time))
		if note.Starred {
			b.WriteString(" *")
		}
		if first != "" {
			b.WriteString(" ")
			b.WriteString(first)
		}
		b.WriteString("\r\n")

		for _, cont := range lines[1:] {
			if cont != "" {
				b.WriteString(continuationIndent)
				b.WriteString(cont)
			}
			b.WriteString("\r\n")
		}
	}

	return []byte(b.String())
}
