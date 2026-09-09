package transcript

import (
	"fmt"
	"strconv"
	"strings"
)

// parseTimecode converts a subtitle timestamp to seconds.
//
// It accepts both the SubRip comma form (00:01:15,200) and the WebVTT dot form
// (00:01:15.200), and both are accepted regardless of which parser is running:
// real-world files mix them, and rejecting a cue over a punctuation choice would
// lose transcript lines for no benefit.
//
// WebVTT also permits the hours field to be omitted (01:15.200), so a two-field
// timecode is read as MM:SS.
func parseTimecode(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty timecode")
	}

	// Normalise the fractional separator so only one form needs handling.
	s = strings.Replace(s, ",", ".", 1)

	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("malformed timecode %q", s)
	}

	var hours, minutes float64
	var secondsField string

	if len(parts) == 3 {
		h, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, fmt.Errorf("malformed hours in %q: %w", s, err)
		}
		m, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, fmt.Errorf("malformed minutes in %q: %w", s, err)
		}
		hours, minutes, secondsField = h, m, parts[2]
	} else {
		m, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, fmt.Errorf("malformed minutes in %q: %w", s, err)
		}
		minutes, secondsField = m, parts[1]
	}

	seconds, err := strconv.ParseFloat(secondsField, 64)
	if err != nil {
		return 0, fmt.Errorf("malformed seconds in %q: %w", s, err)
	}

	total := hours*3600 + minutes*60 + seconds
	if total < 0 {
		return 0, fmt.Errorf("negative timecode %q", s)
	}
	return total, nil
}

// splitCueTiming splits a cue timing line on the "-->" arrow.
//
// The trailing part may carry WebVTT cue settings (align, position, line...),
// which are presentation-only and are discarded here rather than in the tag
// stripper, because they are not part of the text at all.
func splitCueTiming(line string) (start, end string, ok bool) {
	idx := strings.Index(line, "-->")
	if idx < 0 {
		return "", "", false
	}

	start = strings.TrimSpace(line[:idx])
	rest := strings.TrimSpace(line[idx+len("-->"):])

	// The end timecode is the first whitespace-delimited field; anything after
	// it is cue settings.
	if space := strings.IndexFunc(rest, func(r rune) bool { return r == ' ' || r == '\t' }); space >= 0 {
		end = rest[:space]
	} else {
		end = rest
	}

	return start, end, start != "" && end != ""
}
