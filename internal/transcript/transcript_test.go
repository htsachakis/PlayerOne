package transcript

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func nearly(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestParseSRTBasic(t *testing.T) {
	const input = "1\n" +
		"00:00:15,200 --> 00:00:18,100\n" +
		"Example subtitle text.\n" +
		"\n" +
		"2\n" +
		"00:00:18,100 --> 00:00:21,000\n" +
		"Second line.\n"

	got, err := ParseSRT(input)
	if err != nil {
		t.Fatalf("ParseSRT: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}

	if !nearly(got[0].Start, 15.2) || !nearly(got[0].End, 18.1) {
		t.Errorf("entry 0 timing = %v..%v, want 15.2..18.1", got[0].Start, got[0].End)
	}
	if got[0].Text != "Example subtitle text." {
		t.Errorf("entry 0 text = %q", got[0].Text)
	}
	if got[1].Text != "Second line." {
		t.Errorf("entry 1 text = %q", got[1].Text)
	}
}

func TestParseSRTMultilineCueIsJoined(t *testing.T) {
	const input = "1\n" +
		"00:00:00,080 --> 00:00:04,000\n" +
		"If you want your quadcopter to fly as good as\n" +
		"it possibly can, if you're having some kind of\n"

	got, err := ParseSRT(input)
	if err != nil {
		t.Fatalf("ParseSRT: %v", err)
	}
	want := "If you want your quadcopter to fly as good as it possibly can, if you're having some kind of"
	if got[0].Text != want {
		t.Errorf("text = %q,\nwant %q", got[0].Text, want)
	}
}

// The real fixture pads line ends with U+00A0. Those must not survive as gaps.
func TestParseSRTFoldsNonBreakingSpaces(t *testing.T) {
	input := "1\n00:00:00,080 --> 00:00:04,000\nfly as good as\u00a0\nit possibly can\u00a0\u00a0\n"

	got, err := ParseSRT(input)
	if err != nil {
		t.Fatalf("ParseSRT: %v", err)
	}
	if strings.ContainsRune(got[0].Text, '\u00a0') {
		t.Errorf("text still contains U+00A0: %q", got[0].Text)
	}
	if got[0].Text != "fly as good as it possibly can" {
		t.Errorf("text = %q", got[0].Text)
	}
}

func TestParseSRTHandlesCRLF(t *testing.T) {
	const input = "1\r\n00:00:01,000 --> 00:00:02,000\r\nWindows line endings.\r\n\r\n"

	got, err := ParseSRT(input)
	if err != nil {
		t.Fatalf("ParseSRT: %v", err)
	}
	if len(got) != 1 || got[0].Text != "Windows line endings." {
		t.Fatalf("got %+v", got)
	}
}

func TestParseSRTSkipsBOM(t *testing.T) {
	const input = "\ufeff1\n00:00:01,000 --> 00:00:02,000\nWith a byte order mark.\n"

	got, err := ParseSRT(input)
	if err != nil {
		t.Fatalf("ParseSRT: %v", err)
	}
	if got[0].Text != "With a byte order mark." {
		t.Errorf("text = %q", got[0].Text)
	}
}

// Cue indices are ignored, so wrong or missing ones must not shift the output.
func TestParseSRTToleratesBadIndices(t *testing.T) {
	const input = "7\n00:00:01,000 --> 00:00:02,000\nFirst.\n" +
		"\n" +
		"00:00:02,000 --> 00:00:03,000\nSecond, no index.\n" +
		"\n" +
		"7\n00:00:03,000 --> 00:00:04,000\nThird, duplicate index.\n"

	got, err := ParseSRT(input)
	if err != nil {
		t.Fatalf("ParseSRT: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
	}
	for i, want := range []string{"First.", "Second, no index.", "Third, duplicate index."} {
		if got[i].Text != want {
			t.Errorf("entry %d = %q, want %q", i, got[i].Text, want)
		}
	}
}

// Some muxers omit the blank separator entirely. The next cue's index must not
// be absorbed as subtitle text.
func TestParseSRTMissingBlankSeparator(t *testing.T) {
	const input = "1\n00:00:01,000 --> 00:00:02,000\nFirst.\n" +
		"2\n00:00:02,000 --> 00:00:03,000\nSecond.\n"

	got, err := ParseSRT(input)
	if err != nil {
		t.Fatalf("ParseSRT: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].Text != "First." {
		t.Errorf("entry 0 = %q, want %q", got[0].Text, "First.")
	}
	if got[1].Text != "Second." {
		t.Errorf("entry 1 = %q, want %q", got[1].Text, "Second.")
	}
}

func TestParseSRTSkipsMalformedCuesButKeepsGoodOnes(t *testing.T) {
	const input = "1\n00:00:01,000 --> 00:00:02,000\nGood.\n" +
		"\n" +
		"2\nnot a timecode --> also not\nDropped.\n" +
		"\n" +
		"3\n00:00:05,000 --> 00:00:06,000\nAlso good.\n"

	got, err := ParseSRT(input)
	if err != nil {
		t.Fatalf("ParseSRT: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2 (the malformed cue should be skipped)", len(got))
	}
	if got[0].Text != "Good." || got[1].Text != "Also good." {
		t.Errorf("got %+v", got)
	}
}

func TestParseSRTEmptyCueTextIsDropped(t *testing.T) {
	const input = "1\n00:00:01,000 --> 00:00:02,000\n\n" +
		"2\n00:00:02,000 --> 00:00:03,000\nReal text.\n"

	got, err := ParseSRT(input)
	if err != nil {
		t.Fatalf("ParseSRT: %v", err)
	}
	if len(got) != 1 || got[0].Text != "Real text." {
		t.Fatalf("got %+v, want only the cue with text", got)
	}
}

func TestParseSRTErrorsOnNoCues(t *testing.T) {
	if _, err := ParseSRT("just some text\nwith no cues\n"); err == nil {
		t.Fatal("expected an error when there are no cues")
	}
}

func TestParseVTTBasic(t *testing.T) {
	const input = "WEBVTT\n" +
		"\n" +
		"00:00:15.200 --> 00:00:18.100\n" +
		"Example subtitle text.\n" +
		"\n" +
		"00:00:18.100 --> 00:00:21.000\n" +
		"Second line.\n"

	got, err := ParseVTT(input)
	if err != nil {
		t.Fatalf("ParseVTT: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if !nearly(got[0].Start, 15.2) || !nearly(got[0].End, 18.1) {
		t.Errorf("timing = %v..%v, want 15.2..18.1", got[0].Start, got[0].End)
	}
}

func TestParseVTTIgnoresCueSettings(t *testing.T) {
	const input = "WEBVTT\n\n" +
		"00:00:01.000 --> 00:00:02.000 align:start position:10% line:90%\n" +
		"Positioned text.\n"

	got, err := ParseVTT(input)
	if err != nil {
		t.Fatalf("ParseVTT: %v", err)
	}
	if !nearly(got[0].End, 2.0) {
		t.Errorf("End = %v, want 2.0 (cue settings must not corrupt the timecode)", got[0].End)
	}
	if got[0].Text != "Positioned text." {
		t.Errorf("text = %q", got[0].Text)
	}
}

func TestParseVTTSkipsNoteAndStyleBlocks(t *testing.T) {
	const input = "WEBVTT\n" +
		"Kind: captions\n" +
		"Language: en\n" +
		"\n" +
		"NOTE This is a comment\n" +
		"that spans two lines.\n" +
		"\n" +
		"STYLE\n" +
		"::cue { color: yellow }\n" +
		"\n" +
		"00:00:01.000 --> 00:00:02.000\n" +
		"Actual caption.\n"

	got, err := ParseVTT(input)
	if err != nil {
		t.Fatalf("ParseVTT: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(got), got)
	}
	if got[0].Text != "Actual caption." {
		t.Errorf("text = %q", got[0].Text)
	}
}

func TestParseVTTCueIdentifierIsNotText(t *testing.T) {
	const input = "WEBVTT\n\n" +
		"intro-cue\n" +
		"00:00:01.000 --> 00:00:02.000\n" +
		"Hello.\n"

	got, err := ParseVTT(input)
	if err != nil {
		t.Fatalf("ParseVTT: %v", err)
	}
	if len(got) != 1 || got[0].Text != "Hello." {
		t.Fatalf("got %+v, want a single cue reading %q", got, "Hello.")
	}
}

func TestParseVTTShortTimecode(t *testing.T) {
	const input = "WEBVTT\n\n01:15.200 --> 01:18.000\nNo hours field.\n"

	got, err := ParseVTT(input)
	if err != nil {
		t.Fatalf("ParseVTT: %v", err)
	}
	if !nearly(got[0].Start, 75.2) {
		t.Errorf("Start = %v, want 75.2", got[0].Start)
	}
}

func TestParseVTTStripsVoiceAndClassTags(t *testing.T) {
	const input = "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\n" +
		"<v Brian White><c.yellow>Check your CPU load.</c></v>\n"

	got, err := ParseVTT(input)
	if err != nil {
		t.Fatalf("ParseVTT: %v", err)
	}
	if got[0].Text != "Check your CPU load." {
		t.Errorf("text = %q, want %q", got[0].Text, "Check your CPU load.")
	}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Format
	}{
		{"vtt", "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nhi\n", FormatWebVTT},
		{"vtt with bom", "\ufeffWEBVTT\n", FormatWebVTT},
		{"vtt with header text", "WEBVTT - Some title\n", FormatWebVTT},
		{"srt", "1\n00:00:01,000 --> 00:00:02,000\nhi\n", FormatSRT},
		{"empty", "   \n\n", FormatUnknown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detect(tc.in); got != tc.want {
				t.Errorf("Detect = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseDispatchesByFormat(t *testing.T) {
	vtt, err := Parse("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nfrom vtt\n")
	if err != nil {
		t.Fatalf("Parse(vtt): %v", err)
	}
	if vtt[0].Text != "from vtt" {
		t.Errorf("vtt text = %q", vtt[0].Text)
	}

	srt, err := Parse("1\n00:00:01,000 --> 00:00:02,000\nfrom srt\n")
	if err != nil {
		t.Fatalf("Parse(srt): %v", err)
	}
	if srt[0].Text != "from srt" {
		t.Errorf("srt text = %q", srt[0].Text)
	}

	if _, err := Parse("   "); err == nil {
		t.Error("expected an error for empty subtitle data")
	}
}

func TestParseTimecode(t *testing.T) {
	cases := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		{"00:00:00,000", 0, false},
		{"00:00:15,200", 15.2, false},
		{"00:01:15,200", 75.2, false},
		{"01:00:00,000", 3600, false},
		{"02:18:29,237", 8309.237, false},
		{"00:00:15.200", 15.2, false},   // dot form accepted by both parsers
		{"01:15.200", 75.2, false},      // WebVTT short form
		{"1:02:03,500", 3723.5, false},  // single-digit hour
		{"", 0, true},
		{"nonsense", 0, true},
		{"00:00:00:000", 0, true},
		{"aa:bb:cc,ddd", 0, true},
	}

	for _, tc := range cases {
		got, err := parseTimecode(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseTimecode(%q) = %v, want an error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTimecode(%q): %v", tc.in, err)
			continue
		}
		if !nearly(got, tc.want) {
			t.Errorf("parseTimecode(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSplitCueTiming(t *testing.T) {
	start, end, ok := splitCueTiming("00:00:01,000 --> 00:00:02,000")
	if !ok || start != "00:00:01,000" || end != "00:00:02,000" {
		t.Errorf("got %q, %q, %v", start, end, ok)
	}

	start, end, ok = splitCueTiming("00:00:01.000 --> 00:00:02.000 align:start")
	if !ok || end != "00:00:02.000" {
		t.Errorf("cue settings leaked into the end timecode: %q, %q, %v", start, end, ok)
	}

	if _, _, ok := splitCueTiming("no arrow here"); ok {
		t.Error("expected splitCueTiming to reject a line with no arrow")
	}
}

func TestCleanTextStripsPresentationTags(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"italic", "<i>Emphasised</i> text", "Emphasised text"},
		{"bold", "<b>Bold</b> text", "Bold text"},
		{"underline", "<u>Under</u>line", "Under line"},
		{"nested", "<i><b>Both</b></i>", "Both"},
		{"font colour", `<font color="#ffffff">White</font>`, "White"},
		{"vtt class", "<c.yellow>Yellow</c>", "Yellow"},
		{"vtt voice", "<v Brian>Hello</v>", "Hello"},
		{"vtt timestamp", "Karaoke <00:00:01.500>word", "Karaoke word"},
		{"ass override", `{\an8}Top aligned`, "Top aligned"},
		{"ass position", `{\pos(400,570)}Positioned`, "Positioned"},
		{"uppercase tags", "<I>Caps</I>", "Caps"},
		{"self closing br", "line one<br/>line two", "line one line two"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CleanText(tc.in); got != tc.want {
				t.Errorf("CleanText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The whole point of the narrow patterns: tutorial subtitles contain code and
// mathematics, and a greedy stripper would silently eat them.
func TestCleanTextPreservesLegitimatePunctuation(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"less than", "set P to 45 if x < 5", "set P to 45 if x < 5"},
		{"greater than", "throttle > 50%", "throttle > 50%"},
		{"arrow", "PID -> PIDToolbox", "PID -> PIDToolbox"},
		{"comparison pair", "a <-> b", "a <-> b"},
		{"literal braces", "the {count} placeholder", "the {count} placeholder"},
		{"backslash n literal", `print("\n")`, `print("\n")`},
		{"apostrophes", "don't touch it, it's fine", "don't touch it, it's fine"},
		{"ellipsis and dashes", "wait... then — go", "wait... then — go"},
		{"speaker dash", "- Yes. - No.", "- Yes. - No."},
		{"music note", "♪ background music ♪", "♪ background music ♪"},
		{"unknown tag survives", "<sometag>kept", "<sometag>kept"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CleanText(tc.in); got != tc.want {
				t.Errorf("CleanText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Entities are decoded after tags are stripped, so encoded markup survives as
// readable text instead of being deleted as if it were markup.
func TestCleanTextDecodesEntitiesAfterStrippingTags(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Tom &amp; Jerry", "Tom & Jerry"},
		{"&lt;div&gt;", "<div>"},
		{"<i>&lt;i&gt;</i>", "<i>"},
		{"&quot;quoted&quot;", `"quoted"`},
		{"it&#39;s", "it's"},
		{"a&nbsp;b", "a b"},
	}

	for _, tc := range cases {
		if got := CleanText(tc.in); got != tc.want {
			t.Errorf("CleanText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCleanTextEmpty(t *testing.T) {
	if got := CleanText(""); got != "" {
		t.Errorf("CleanText(\"\") = %q", got)
	}
	if got := CleanText("<i></i>"); got != "" {
		t.Errorf("CleanText of tags only = %q, want empty", got)
	}
}

func TestCleanEntriesDropsEmpties(t *testing.T) {
	in := []Entry{
		{Start: 0, End: 1, Text: "<i>keep</i>"},
		{Start: 1, End: 2, Text: "{\\an8}"},
		{Start: 2, End: 3, Text: "also keep"},
	}

	got := CleanEntries(in)
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(got), got)
	}
	if got[0].Text != "keep" || got[1].Text != "also keep" {
		t.Errorf("got %+v", got)
	}
	// The input must not be corrupted by the in-place-looking slice trick.
	if in[1].Text != "{\\an8}" {
		t.Errorf("CleanEntries mutated its input: %+v", in)
	}
}

// Parsing the real 2h18m subtitle file is the strongest signal that the parser
// survives contact with genuine, machine-generated subtitles.
func TestParseRealFixture(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "..", "testdata", "media", "*.srt"))
	if err != nil {
		t.Fatalf("globbing fixtures: %v", err)
	}
	if len(matches) == 0 {
		t.Skip("no .srt fixture in testdata/media; see README for where to put test media")
	}

	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	entries, err := Parse(string(raw))
	if err != nil {
		t.Fatalf("Parse(fixture): %v", err)
	}
	if len(entries) < 100 {
		t.Fatalf("got %d entries from the fixture, expected many more", len(entries))
	}

	var lastEnd float64
	for i, e := range entries {
		if e.Text == "" {
			t.Fatalf("entry %d has empty text", i)
		}
		if e.End < e.Start {
			t.Fatalf("entry %d ends (%v) before it starts (%v)", i, e.End, e.Start)
		}
		if e.Start < lastEnd-0.001 {
			t.Fatalf("entry %d starts at %v, before the previous cue ended at %v", i, e.Start, lastEnd)
		}
		if strings.ContainsRune(e.Text, '\u00a0') {
			t.Fatalf("entry %d still contains U+00A0: %q", i, e.Text)
		}
		if strings.Contains(e.Text, "  ") {
			t.Fatalf("entry %d contains a double space: %q", i, e.Text)
		}
		lastEnd = e.End
	}

	t.Logf("parsed %d transcript entries spanning %.0f seconds", len(entries), lastEnd)
}
