package notes

import (
	"strings"
	"testing"
)

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name string
		line string
		want float64
		rest string
		ok   bool
	}{
		{"minutes and seconds", "12:34 hello", 754, "hello", true},
		{"hours", "1:02:03 hello", 3723, "hello", true},
		{"padded hours", "01:02:03 hello", 3723, "hello", true},
		{"no text", "00:12:34", 754, "", true},
		{"fractional", "00:00:01.500 hi", 1.5, "hi", true},
		{"tab separated", "12:34\ttext", 754, "text", true},
		{"minutes over sixty", "90:00 long", 5400, "long", true},
		{"plain text", "hello there", 0, "", false},
		{"leading digits only", "42 is the answer", 0, "", false},
		{"empty", "", 0, "", false},
		{"malformed", "12::34 x", 0, "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, rest, ok := parseTimestamp(tc.line)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if got != tc.want {
				t.Errorf("time = %v, want %v", got, tc.want)
			}
			if rest != tc.rest {
				t.Errorf("rest = %q, want %q", rest, tc.rest)
			}
		})
	}
}

func TestParse(t *testing.T) {
	input := strings.Join([]string{
		"# PlayerOne notes - lesson.mp4",
		"",
		"00:00:42",
		"00:12:34 * Closures capture the variable",
		"00:18:05 The bit about defer,",
		"         it is the second example",
		"",
		"         that actually shows it",
	}, "\n")

	got, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d notes, want 3: %#v", len(got), got)
	}

	if got[0].Time != 42 || got[0].Text != "" || got[0].Starred {
		t.Errorf("bookmark = %#v", got[0])
	}
	if got[1].Time != 754 || got[1].Text != "Closures capture the variable" || !got[1].Starred {
		t.Errorf("starred note = %#v", got[1])
	}

	wantText := "The bit about defer,\nit is the second example\n\nthat actually shows it"
	if got[2].Time != 1085 || got[2].Text != wantText {
		t.Errorf("multi-line note = %#v, want text %q", got[2], wantText)
	}
}

func TestParseIsLenient(t *testing.T) {
	input := strings.Join([]string{
		"random preamble nobody meant to be a note",
		"18:05 out of order",
		"12:34 earlier",
		"unindented continuation",
	}, "\r\n")

	got, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d notes, want 2: %#v", len(got), got)
	}
	// Sorted by time, so the 12:34 note comes first even though it was written
	// second.
	if got[0].Time != 754 {
		t.Errorf("first note time = %v, want 754", got[0].Time)
	}
	// A continuation belongs to the note above it in the file, which is 12:34,
	// not to whichever note sorts before it afterwards.
	if got[0].Text != "earlier\nunindented continuation" {
		t.Errorf("continuation = %q", got[0].Text)
	}
	if got[1].Text != "out of order" {
		t.Errorf("second note = %q", got[1].Text)
	}
}

func TestParseLiteralAsterisk(t *testing.T) {
	got, err := Parse(strings.NewReader(`12:34 \*not starred`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d notes, want 1", len(got))
	}
	if got[0].Starred {
		t.Error("note should not be starred")
	}
	if got[0].Text != "*not starred" {
		t.Errorf("text = %q, want %q", got[0].Text, "*not starred")
	}
}

func TestRoundTrip(t *testing.T) {
	want := []Note{
		{Time: 42},                                                        // a bookmark
		{Time: 754, Text: "Closures capture the variable", Starred: true}, // starred
		{Time: 1085, Text: "First line\nsecond line\n\nafter a gap"},      // multi-line
		{Time: 2000, Text: "*literally starts with an asterisk"},          // escaped
		{Time: 3723, Text: "Ünicode — em dash and ünlaut", Starred: true},
	}

	got, err := Parse(strings.NewReader(string(Format(want, "lesson.mp4"))))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d notes, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("note %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestFormatSortsAndUsesCRLF(t *testing.T) {
	out := string(Format([]Note{{Time: 100, Text: "later"}, {Time: 10, Text: "earlier"}}, "x.mp4"))

	if !strings.Contains(out, "\r\n") {
		t.Error("expected CRLF line endings")
	}
	if strings.Index(out, "earlier") > strings.Index(out, "later") {
		t.Error("notes were not sorted by timestamp")
	}
	if !strings.HasPrefix(out, "# PlayerOne notes - x.mp4") {
		t.Errorf("missing header: %q", out)
	}
}
