package transcript

import (
	"fmt"
	"strings"
)

// ParseSRT parses SubRip subtitle text.
//
// The structure is: an optional numeric index line, a timing line containing
// "-->", then one or more text lines, then a blank line. The index is ignored
// entirely — it is redundant, and files produced by muxing tools frequently
// number cues wrongly or not at all.
func ParseSRT(text string) ([]Entry, error) {
	lines := strings.Split(normaliseNewlines(text), "\n")

	var (
		entries  []Entry
		malformed int
	)

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// A cue is located by its timing line rather than by counting from the
		// index, so a missing or duplicated index cannot desynchronise parsing.
		if !strings.Contains(line, "-->") {
			continue
		}

		startText, endText, ok := splitCueTiming(line)
		if !ok {
			malformed++
			continue
		}

		start, err := parseTimecode(startText)
		if err != nil {
			malformed++
			continue
		}
		end, err := parseTimecode(endText)
		if err != nil {
			malformed++
			continue
		}

		// Consume text lines up to the next blank line or the next timing line.
		// Stopping at a timing line matters for files whose blank-line
		// separators are missing.
		var body []string
		for i+1 < len(lines) {
			next := lines[i+1]
			if strings.TrimSpace(next) == "" {
				i++
				break
			}
			if strings.Contains(next, "-->") {
				break
			}
			body = append(body, next)
			i++
		}

		// A numeric index belonging to the *next* cue can be captured when the
		// blank separator is missing; drop a trailing bare number.
		body = dropTrailingIndex(body, lines, i)

		content := CleanText(joinCueLines(body))
		if content == "" {
			continue // a timing with no text carries nothing for a transcript
		}

		entries = append(entries, Entry{Start: start, End: end, Text: content})
	}

	if len(entries) == 0 {
		if malformed > 0 {
			return nil, fmt.Errorf("transcript: no readable cues (%d malformed timings)", malformed)
		}
		return nil, fmt.Errorf("transcript: no subtitle cues found")
	}

	return entries, nil
}

// dropTrailingIndex removes a bare number at the end of a cue body when the
// following line is a timing line, which means the number was the next cue's
// index rather than subtitle text.
func dropTrailingIndex(body []string, lines []string, i int) []string {
	if len(body) == 0 {
		return body
	}

	last := strings.TrimSpace(body[len(body)-1])
	if !isAllDigits(last) {
		return body
	}
	if i+1 < len(lines) && strings.Contains(lines[i+1], "-->") {
		return body[:len(body)-1]
	}
	return body
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
