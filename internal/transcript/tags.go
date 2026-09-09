package transcript

import (
	"html"
	"regexp"
	"strings"
)

// Presentation markup is stripped with narrowly-targeted patterns rather than a
// blanket "delete everything between angle brackets".
//
// That distinction matters for this application specifically: tutorial subtitles
// contain code and mathematics, so "if x < 5" and "a <-> b" and "{count}" are
// ordinary text that a greedy stripper would silently destroy. Only markup that
// is unambiguously presentational is removed.
var (
	// HTML and WebVTT styling tags, by name. <font color="#fff">, <c.yellow>
	// and the WebVTT voice tag <v Speaker> all match.
	// Longer names are listed first so the intended alternative is the one that
	// matches, independent of the regexp engine's preference rules.
	htmlTagPattern = regexp.MustCompile(`(?i)</?(?:strong|span|ruby|lang|font|br|em|rp|rt|i|b|u|s|c|v)(?:[.:][^>\s]*)?(?:\s[^>]*)?/?>`)

	// WebVTT inline timestamp tags, used for karaoke-style highlighting:
	// <00:01:15.200>.
	vttTimestampPattern = regexp.MustCompile(`<\d{1,3}:\d{2}(?::\d{2})?[.,]\d{1,3}>`)

	// ASS/SSA override blocks. The leading backslash is required, so a literal
	// "{brace}" in the dialogue survives.
	assOverridePattern = regexp.MustCompile(`\{\s*\\[^}]*\}`)
)

// CleanText removes presentation markup from one line of subtitle text while
// preserving the words and punctuation a reader needs.
//
// Entities are decoded only after tags are stripped, so text that arrived as
// "&lt;div&gt;" ends up as literal "<div>" rather than being mistaken for markup
// and deleted. That ordering is what makes transcripts of programming tutorials
// readable.
func CleanText(s string) string {
	if s == "" {
		return ""
	}

	s = assOverridePattern.ReplaceAllString(s, "")
	s = vttTimestampPattern.ReplaceAllString(s, "")
	s = htmlTagPattern.ReplaceAllString(s, " ")

	s = html.UnescapeString(s)

	return collapseSpaces(s)
}

// CleanEntries applies CleanText across a slice, dropping entries left empty.
// Used when subtitle text arrives already parsed, such as from a caller that
// built entries itself.
func CleanEntries(entries []Entry) []Entry {
	out := entries[:0:0]
	for _, e := range entries {
		e.Text = CleanText(e.Text)
		if strings.TrimSpace(e.Text) == "" {
			continue
		}
		out = append(out, e)
	}
	return out
}
