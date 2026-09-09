package transcript

import (
	"fmt"
	"strings"
)

// ParseVTT parses WebVTT subtitle text.
//
// WebVTT differs from SubRip in ways that matter here: it opens with a "WEBVTT"
// header, cues may carry an identifier line before the timing, the file may
// contain NOTE / STYLE / REGION blocks that are not cues at all, and the timing
// line may be followed by cue settings.
func ParseVTT(text string) ([]Entry, error) {
	lines := strings.Split(normaliseNewlines(text), "\n")

	var (
		entries   []Entry
		malformed int
	)

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Skip non-cue blocks wholesale. They can contain arbitrary text, so
		// scanning through them line by line risks misreading their content.
		if isBlockHeader(line) {
			for i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
				i++
			}
			continue
		}

		if !strings.Contains(line, "-->") {
			continue // header, cue identifier, or stray text
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

		content := CleanText(joinCueLines(body))
		if content == "" {
			continue
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

// isBlockHeader reports whether a line opens a WebVTT block that is not a cue.
//
// The magic "WEBVTT" line is included: it may carry trailing text, and anything
// following it up to the first blank line is header metadata.
func isBlockHeader(line string) bool {
	for _, keyword := range []string{"WEBVTT", "NOTE", "STYLE", "REGION"} {
		if line == keyword || strings.HasPrefix(line, keyword+" ") || strings.HasPrefix(line, keyword+"\t") {
			return true
		}
	}
	return false
}
