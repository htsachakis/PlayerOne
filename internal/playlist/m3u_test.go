package playlist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAndReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "course"+Extension)

	items := []Item{
		{Path: filepath.Join(dir, "lesson1.mp4"), Filename: "lesson1.mp4", Title: "Lesson one", Duration: 61.4},
		{Path: filepath.Join(dir, "lesson2.mp4"), Filename: "lesson2.mp4", Title: "Lesson two", Duration: 0},
	}

	if err := WriteM3U(file, items); err != nil {
		t.Fatalf("WriteM3U: %v", err)
	}

	got, err := ReadM3U(file)
	if err != nil {
		t.Fatalf("ReadM3U: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}

	if got[0].Title != "Lesson one" {
		t.Errorf("title = %q, want %q", got[0].Title, "Lesson one")
	}
	if got[0].Duration != 61 {
		t.Errorf("duration = %v, want 61 (rounded on save)", got[0].Duration)
	}
	if !strings.EqualFold(got[0].Path, items[0].Path) {
		t.Errorf("path = %q, want %q", got[0].Path, items[0].Path)
	}
	if got[1].Duration != 0 {
		t.Errorf("an unknown duration should stay unknown, got %v", got[1].Duration)
	}
}

func TestWrittenFileIsValidM3U8(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "list"+Extension)

	if err := WriteM3U(file, []Item{
		{Path: `C:\Videos\a.mkv`, Filename: "a.mkv", Title: "A", Duration: 10},
	}); err != nil {
		t.Fatalf("WriteM3U: %v", err)
	}

	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	text := string(raw)

	if !strings.HasPrefix(text, "#EXTM3U") {
		t.Errorf("the file does not start with #EXTM3U:\n%s", text)
	}
	if !strings.Contains(text, "#EXTINF:10,A") {
		t.Errorf("the EXTINF line is wrong:\n%s", text)
	}
	if !strings.Contains(text, `C:\Videos\a.mkv`) {
		t.Errorf("the path is missing:\n%s", text)
	}
}

// A playlist saved beside its media, then moved with it, must still work.
func TestRelativePathsResolveAgainstTheFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "relative.m3u8")

	content := "#EXTM3U\n#EXTINF:12,First\nlesson1.mp4\n#EXTINF:-1,Second\nsub/lesson2.mp4\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("writing: %v", err)
	}

	got, err := ReadM3U(file)
	if err != nil {
		t.Fatalf("ReadM3U: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}

	want := filepath.Join(dir, "lesson1.mp4")
	if !strings.EqualFold(got[0].Path, want) {
		t.Errorf("path = %q, want %q", got[0].Path, want)
	}
	if !filepath.IsAbs(got[1].Path) {
		t.Errorf("path %q was not made absolute", got[1].Path)
	}
}

func TestReadHandlesAPlainListWithNoDirectives(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "plain.m3u")

	content := "a.mkv\nb.mkv\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("writing: %v", err)
	}

	got, err := ReadM3U(file)
	if err != nil {
		t.Fatalf("ReadM3U: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].Filename != "a.mkv" {
		t.Errorf("filename = %q", got[0].Filename)
	}
}

// Playlists written by other tools frequently carry a byte order mark, which
// would otherwise become part of the first path.
func TestReadSkipsAByteOrderMark(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bom.m3u8")

	content := "\ufeff#EXTM3U\nlesson1.mp4\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("writing: %v", err)
	}

	got, err := ReadM3U(file)
	if err != nil {
		t.Fatalf("ReadM3U: %v", err)
	}
	if got[0].Filename != "lesson1.mp4" {
		t.Errorf("filename = %q, want the mark stripped", got[0].Filename)
	}
}

func TestReadHandlesCRLF(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "crlf.m3u8")

	if err := os.WriteFile(file, []byte("#EXTM3U\r\n#EXTINF:5,Title\r\na.mkv\r\n"), 0o644); err != nil {
		t.Fatalf("writing: %v", err)
	}

	got, err := ReadM3U(file)
	if err != nil {
		t.Fatalf("ReadM3U: %v", err)
	}
	if got[0].Title != "Title" {
		t.Errorf("title = %q, want %q", got[0].Title, "Title")
	}
	if got[0].Filename != "a.mkv" {
		t.Errorf("filename = %q", got[0].Filename)
	}
}

// A remote entry is skipped rather than queued: handing a URL to mpv is a
// different feature with different consequences.
func TestReadSkipsRemoteEntries(t *testing.T) {
	items, err := parseM3U(strings.NewReader(
		"#EXTM3U\nhttps://example.invalid/stream.m3u8\n#EXTINF:5,Local\nlocal.mkv\n"), `C:\media`)
	if err != nil {
		t.Fatalf("parseM3U: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d entries, want only the local one: %+v", len(items), items)
	}
	if items[0].Filename != "local.mkv" {
		t.Errorf("filename = %q", items[0].Filename)
	}
}

// A title belongs to the entry that follows it; a skipped remote entry must not
// leave its title attached to the next one.
func TestSkippedEntryDoesNotStealTheNextTitle(t *testing.T) {
	items, err := parseM3U(strings.NewReader(
		"#EXTM3U\n#EXTINF:5,Remote title\nhttps://example.invalid/a.mkv\nlocal.mkv\n"), `C:\media`)
	if err != nil {
		t.Fatalf("parseM3U: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d entries, want 1", len(items))
	}
	if items[0].Title != "" {
		t.Errorf("title = %q, want it not carried over from the skipped entry", items[0].Title)
	}
}

func TestReadDropsDuplicates(t *testing.T) {
	items, err := parseM3U(strings.NewReader("a.mkv\nA.MKV\nb.mkv\n"), `C:\media`)
	if err != nil {
		t.Fatalf("parseM3U: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("got %d entries, want 2 - the repeat should be dropped", len(items))
	}
}

func TestReadRejectsAnEmptyPlaylist(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "empty.m3u8")
	if err := os.WriteFile(file, []byte("#EXTM3U\n# nothing here\n"), 0o644); err != nil {
		t.Fatalf("writing: %v", err)
	}

	if _, err := ReadM3U(file); err == nil {
		t.Error("expected an error for a playlist with no entries")
	}
}

func TestParseExtInf(t *testing.T) {
	cases := []struct {
		in      string
		seconds float64
		title   string
	}{
		{"#EXTINF:123,Some title", 123, "Some title"},
		{"#EXTINF:-1,Unknown length", 0, "Unknown length"},
		{"#EXTINF:0,Zero", 0, "Zero"},
		{"#EXTINF:61.5,Fractional", 61.5, "Fractional"},
		{"#EXTINF:12,", 12, ""},
		{"#EXTINF:12", 12, ""},
		{"#EXTINF:12,Title, with a comma", 12, "Title, with a comma"},
	}

	for _, tc := range cases {
		secs, title := parseExtInf(tc.in)
		if secs != tc.seconds || title != tc.title {
			t.Errorf("parseExtInf(%q) = %v, %q; want %v, %q", tc.in, secs, title, tc.seconds, tc.title)
		}
	}
}

// A newline in a title would otherwise forge an extra entry when written out.
func TestWriteNeutralisesNewlinesInTitles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "odd"+Extension)

	if err := WriteM3U(file, []Item{
		{Path: filepath.Join(dir, "a.mkv"), Filename: "a.mkv", Title: "Bad\ntitle\r\nC:\\evil.mkv"},
	}); err != nil {
		t.Fatalf("WriteM3U: %v", err)
	}

	got, err := ReadM3U(file)
	if err != nil {
		t.Fatalf("ReadM3U: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1 - a title must not be able to add one", len(got))
	}
}
