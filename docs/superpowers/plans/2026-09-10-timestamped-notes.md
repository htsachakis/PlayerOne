# Timestamped Notes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the viewer press `T` to file a timestamped note (or a textless bookmark) against the moment they are watching, stored in a portable plain-text `<video>.notes` file beside the video.

**Architecture:** A new `internal/notes` package owns the file format and a store that binds one `.notes` file to the open video. `api_notes.go` exposes it to the frontend in the style of the other `api_*.go` files, and a 2-second poll picks up edits made outside the app. The frontend gains a `NotesTab` modelled on `TranscriptTab`, a small composer overlay, and note ticks on the existing timeline.

**Tech Stack:** Go 1.x (standard library only), Wails v2 bindings, TypeScript with no framework, plain CSS.

**Spec:** `docs/superpowers/specs/2026-09-10-timestamped-notes-design.md`

---

## File Structure

**Created:**

| Path | Responsibility |
|---|---|
| `internal/notes/notes.go` | The `Note` type, `Parse`, `Format`. The file format and nothing else. |
| `internal/notes/notes_test.go` | Format round-trips and hand-edited input. |
| `internal/notes/store.go` | Binding a `.notes` file to a video, mutations, atomic save, reload detection. |
| `internal/notes/store_test.go` | Discovery, mutation, unsaved state, reload. |
| `api_notes.go` | The Wails-facing methods. |
| `frontend/src/components/NotesTab.ts` | The Notes panel: list, search, star filter. |
| `frontend/src/components/NoteComposer.ts` | The capture overlay. |

**Modified:**

| Path | Change |
|---|---|
| `internal/timefmt/timefmt.go` | Add `FormatClock` (always `HH:MM:SS`). |
| `internal/history/history.go` | Add `Entry.NotesPath` and `SetNotesPath`. |
| `internal/settings/settings.go` | Add three note settings, defaults, normalisation. |
| `app.go` | Add the `notes` store field, bind on open, run the reload poll. |
| `api_media.go` | Bind the notes store inside `Open`. |
| `frontend/src/types/media.ts` | `Note`, `NotesResult`, `NotesOrigin`. |
| `frontend/src/state/store.ts` | Notes state, `'notes'` panel tab. |
| `frontend/src/services/player.ts` | Notes calls and the `notes:changed` event. |
| `frontend/src/components/SidePanel.ts` | The Notes tab. |
| `frontend/src/components/Timeline.ts` | Note ticks. |
| `frontend/src/keyboard/shortcuts.ts` | `T` opens the composer. |
| `frontend/src/styles/panel.css` | Notes list, star, unsaved bar. |
| `frontend/src/styles/controls.css` | Composer overlay, timeline note marks. |
| `frontend/src/main.ts` | Mount the composer. |
| `README.md`, `docs/architecture.md` | Document the feature and the format. |

---

## Task 1: The file format — parsing

**Files:**
- Create: `internal/notes/notes.go`
- Create: `internal/notes/notes_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/notes/notes_test.go`:

```go
package notes

import (
	"strings"
	"testing"
)

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    float64
		rest    string
		ok      bool
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
	// Sorted by time, so the 12:34 note comes first.
	if got[0].Time != 754 {
		t.Errorf("first note time = %v, want 754", got[0].Time)
	}
	if got[1].Text != "out of order\nunindented continuation" {
		t.Errorf("continuation = %q", got[1].Text)
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/notes/ -run TestParse -v`
Expected: FAIL — the package does not compile, `undefined: parseTimestamp`.

- [ ] **Step 3: Write the implementation**

Create `internal/notes/notes.go`:

```go
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
```

- [ ] **Step 4: Add `FormatClock` to `internal/timefmt/timefmt.go`**

Append to `internal/timefmt/timefmt.go`:

```go
// FormatClock renders a duration as a fully padded HH:MM:SS.
//
// Unlike Format and FormatPadded, which drop the hours field when it is zero to
// keep the interface tidy, this one is fixed-width. It is what the .notes file
// is written with, where a straight column matters more than brevity and a
// parser has to read it back.
func FormatClock(seconds float64) string {
	if !(seconds > 0) { // also catches NaN
		seconds = 0
	}

	total := int64(seconds)
	return fmt.Sprintf("%02d:%02d:%02d", total/3600, (total%3600)/60, total%60)
}
```

And add to `internal/timefmt/timefmt_test.go`:

```go
func TestFormatClock(t *testing.T) {
	tests := []struct {
		seconds float64
		want    string
	}{
		{0, "00:00:00"},
		{-5, "00:00:00"},
		{42, "00:00:42"},
		{754, "00:12:34"},
		{3723, "01:02:03"},
		{360000, "100:00:00"},
	}

	for _, tc := range tests {
		if got := FormatClock(tc.seconds); got != tc.want {
			t.Errorf("FormatClock(%v) = %q, want %q", tc.seconds, got, tc.want)
		}
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/notes/ ./internal/timefmt/ -v`
Expected: PASS.

- [ ] **Step 6: Add the round-trip test**

Append to `internal/notes/notes_test.go`:

```go
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
```

- [ ] **Step 7: Run the tests**

Run: `go test ./internal/notes/ -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/notes/ internal/timefmt/
git commit -m "Read and write the plain-text notes file format"
```

---

## Task 2: The notes store

**Files:**
- Create: `internal/notes/store.go`
- Create: `internal/notes/store_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/notes/store_test.go`:

```go
package notes

import (
	"os"
	"path/filepath"
	"testing"
)

// writeVideo creates a stand-in video file and returns its path.
func writeVideo(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("not really a video"), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func TestBindFindsFileBesideVideo(t *testing.T) {
	dir := t.TempDir()
	video := writeVideo(t, dir, "lesson.mp4")
	if err := os.WriteFile(video+Extension, []byte("12:34 already here\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewStore()
	s.Bind(video)

	if s.Origin() != OriginBeside {
		t.Errorf("origin = %q, want %q", s.Origin(), OriginBeside)
	}
	if s.Path() != video+Extension {
		t.Errorf("path = %q, want %q", s.Path(), video+Extension)
	}
	if list := s.List(); len(list) != 1 || list[0].Text != "already here" {
		t.Errorf("list = %#v", list)
	}
}

func TestBindFallsBackToStemName(t *testing.T) {
	dir := t.TempDir()
	video := writeVideo(t, dir, "lesson.mp4")
	stem := filepath.Join(dir, "lesson"+Extension)
	if err := os.WriteFile(stem, []byte("00:10 hand written\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewStore()
	s.Bind(video)

	if s.Path() != stem {
		t.Errorf("path = %q, want %q", s.Path(), stem)
	}
	if len(s.List()) != 1 {
		t.Errorf("list = %#v", s.List())
	}
}

func TestBindWithNoFileDoesNotCreateOne(t *testing.T) {
	dir := t.TempDir()
	video := writeVideo(t, dir, "lesson.mp4")

	s := NewStore()
	s.Bind(video)

	if s.Path() != video+Extension {
		t.Errorf("path = %q, want %q", s.Path(), video+Extension)
	}
	if len(s.List()) != 0 {
		t.Errorf("list = %#v, want empty", s.List())
	}
	if _, err := os.Stat(video + Extension); !os.IsNotExist(err) {
		t.Error("binding created the file; it should wait for a note")
	}
}

func TestAddWritesTheFile(t *testing.T) {
	dir := t.TempDir()
	video := writeVideo(t, dir, "lesson.mp4")

	s := NewStore()
	s.Bind(video)

	if err := s.Add(754.4, "a note", false); err != nil {
		t.Fatalf("Add: %v", err)
	}

	raw, err := os.ReadFile(video + Extension)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if want := "00:12:34 a note"; !contains(string(raw), want) {
		t.Errorf("file = %q, want it to contain %q", raw, want)
	}
	// Capture rounds to whole seconds so memory and disk cannot drift apart.
	if got := s.List()[0].Time; got != 754 {
		t.Errorf("stored time = %v, want 754", got)
	}
}

func TestAddNearAnExistingNoteReplacesIt(t *testing.T) {
	dir := t.TempDir()
	video := writeVideo(t, dir, "lesson.mp4")

	s := NewStore()
	s.Bind(video)

	if err := s.Add(754, "first", false); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(754.3, "second", true); err != nil {
		t.Fatal(err)
	}

	list := s.List()
	if len(list) != 1 {
		t.Fatalf("got %d notes, want 1: %#v", len(list), list)
	}
	if list[0].Text != "second" || !list[0].Starred {
		t.Errorf("note = %#v", list[0])
	}
}

func TestDeleteAndToggleStar(t *testing.T) {
	dir := t.TempDir()
	video := writeVideo(t, dir, "lesson.mp4")

	s := NewStore()
	s.Bind(video)
	if err := s.Add(754, "a note", false); err != nil {
		t.Fatal(err)
	}

	if err := s.ToggleStar(754); err != nil {
		t.Fatal(err)
	}
	if !s.List()[0].Starred {
		t.Error("note should be starred")
	}

	if err := s.Delete(754); err != nil {
		t.Fatal(err)
	}
	if len(s.List()) != 0 {
		t.Errorf("list = %#v, want empty", s.List())
	}
}

func TestAtToleratesFloatDrift(t *testing.T) {
	s := NewStore()
	s.Bind(filepath.Join(t.TempDir(), "lesson.mp4"))
	if err := s.Add(754, "a note", false); err != nil {
		t.Fatal(err)
	}

	if _, ok := s.At(754.0000001); !ok {
		t.Error("a sub-millisecond difference should still match")
	}
	if _, ok := s.At(755); ok {
		t.Error("a whole second away should not match")
	}
}

func TestUnwritableFolderKeepsNotesInMemory(t *testing.T) {
	// A path inside a file is never writable on any platform, which is a
	// portable way to provoke the failure a read-only folder causes.
	dir := t.TempDir()
	blocker := writeVideo(t, dir, "blocker")
	video := filepath.Join(blocker, "lesson.mp4")

	s := NewStore()
	s.Bind(video)

	err := s.Add(754, "a note", false)
	if err == nil {
		t.Fatal("Add should report that it could not write")
	}
	if s.Origin() != OriginUnsaved {
		t.Errorf("origin = %q, want %q", s.Origin(), OriginUnsaved)
	}
	if len(s.List()) != 1 {
		t.Errorf("the note must survive a failed write: %#v", s.List())
	}
}

func TestSaveAsRebindsAndWrites(t *testing.T) {
	dir := t.TempDir()
	s := NewStore()
	s.Bind(filepath.Join(dir, "lesson.mp4"))
	_ = s.Add(754, "a note", false)

	target := filepath.Join(dir, "elsewhere.notes")
	if err := s.SaveAs(target); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}

	if s.Path() != target {
		t.Errorf("path = %q, want %q", s.Path(), target)
	}
	if s.Origin() != OriginChosen {
		t.Errorf("origin = %q, want %q", s.Origin(), OriginChosen)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("file was not written: %v", err)
	}
}

func TestReloadPicksUpExternalEdits(t *testing.T) {
	dir := t.TempDir()
	video := writeVideo(t, dir, "lesson.mp4")

	s := NewStore()
	s.Bind(video)
	if err := s.Add(754, "mine", false); err != nil {
		t.Fatal(err)
	}

	// Somebody edits the file in Notepad.
	if err := os.WriteFile(video+Extension, []byte("00:12:34 theirs\r\n00:20:00 and another\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := s.Reload()
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if !changed {
		t.Fatal("Reload did not notice the change")
	}
	if list := s.List(); len(list) != 2 || list[0].Text != "theirs" {
		t.Errorf("list = %#v", list)
	}

	// A second reload with nothing changed must be quiet.
	changed, err = s.Reload()
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if changed {
		t.Error("Reload reported a change when the file was untouched")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/notes/ -run TestBind -v`
Expected: FAIL — `undefined: NewStore`.

- [ ] **Step 3: Write the implementation**

Create `internal/notes/store.go`:

```go
package notes

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"playerone/internal/appdir"
)

// Origin says where the bound notes file came from, which is what the interface
// needs in order to explain itself.
type Origin string

const (
	// OriginBeside means the file sits next to the video, found or to be created.
	OriginBeside Origin = "beside"
	// OriginChosen means the user picked the location, either through Load notes
	// or after a write beside the video failed.
	OriginChosen Origin = "chosen"
	// OriginUnsaved means there is nowhere to write yet and the notes exist only
	// in memory.
	OriginUnsaved Origin = "unsaved"
)

// matchTolerance is how close a lookup has to be to count as the same note.
//
// Times make a round trip through JSON and a text file, so exact float equality
// would miss. Captured times are whole seconds, so a millisecond is generous.
const matchTolerance = 0.001

// editWindow is how near an existing note a new capture has to be before it
// edits that note instead of creating a second one.
//
// Pressing the capture key at a spot you already marked should let you add to
// what is there, and it keeps the timestamp usable as the note's identity.
const editWindow = 0.5

// stamp identifies a version of a file on disk without reading it.
type stamp struct {
	size    int64
	modTime int64
}

// Store holds the notes for the open video and the file they belong to.
//
// It is safe for concurrent use: the reload poll and the interface both reach it.
type Store struct {
	mu sync.RWMutex

	path      string
	origin    Origin
	videoName string
	notes     []Note
	seen      stamp
}

// NewStore returns an empty store, bound to nothing.
func NewStore() *Store {
	return &Store{origin: OriginUnsaved}
}

// Bind attaches the store to a video, loading the notes file beside it.
//
// Two names are accepted: lesson.mp4.notes, which is what PlayerOne writes, and
// lesson.notes, which is what somebody creating one by hand would type. Neither
// existing is not a failure - the file is created when there is a note to put
// in it, not before, so merely opening a video never litters the folder.
func (s *Store) Bind(videoPath string) {
	beside := videoPath + Extension
	stem := strings.TrimSuffix(videoPath, filepath.Ext(videoPath)) + Extension

	target := beside
	if !exists(beside) && exists(stem) {
		target = stem
	}

	s.mu.Lock()
	s.path = target
	s.origin = OriginBeside
	s.videoName = filepath.Base(videoPath)
	s.notes = nil
	s.seen = stamp{}
	s.mu.Unlock()

	_, _ = s.Reload()
}

// BindTo attaches the store to a notes file the user chose, reading it if it is
// there. Used by Load notes, and when restoring a remembered binding.
func (s *Store) BindTo(notesPath, videoName string) error {
	s.mu.Lock()
	s.path = notesPath
	s.origin = OriginChosen
	s.videoName = videoName
	s.notes = nil
	s.seen = stamp{}
	s.mu.Unlock()

	_, err := s.Reload()
	return err
}

// Release unbinds the store, which is what closing a file amounts to.
func (s *Store) Release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path, s.origin, s.videoName, s.notes, s.seen = "", OriginUnsaved, "", nil, stamp{}
}

// Path is the bound file, empty when there is nowhere to write.
func (s *Store) Path() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.path
}

// Origin says where the bound file came from.
func (s *Store) Origin() Origin {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.origin
}

// List returns a copy of the notes, sorted by time.
func (s *Store) List() []Note {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Note(nil), s.notes...)
}

// At returns the note at a time, matching within a millisecond.
func (s *Store) At(seconds float64) (Note, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if i := indexAt(s.notes, seconds, matchTolerance); i >= 0 {
		return s.notes[i], true
	}
	return Note{}, false
}

// Nearby returns the note a capture at this time would edit, if there is one.
func (s *Store) Nearby(seconds float64) (Note, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if i := indexAt(s.notes, seconds, editWindow); i >= 0 {
		return s.notes[i], true
	}
	return Note{}, false
}

// Add stores a note, replacing one already within half a second.
//
// The time is rounded to whole seconds, which is the resolution the file is
// written at. Rounding here rather than on write means the note in memory and
// the note on disk are the same note, so re-reading the file never appears to
// move it.
//
// A failed write is returned but does not lose the note: it stays in memory and
// the store falls to OriginUnsaved so the interface can offer somewhere to put
// it.
func (s *Store) Add(seconds float64, text string, starred bool) error {
	seconds = math.Round(math.Max(seconds, 0))
	text = strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n \t")

	s.mu.Lock()
	if i := indexAt(s.notes, seconds, editWindow); i >= 0 {
		s.notes[i] = Note{Time: seconds, Text: text, Starred: starred}
	} else {
		s.notes = append(s.notes, Note{Time: seconds, Text: text, Starred: starred})
		sort.SliceStable(s.notes, func(a, b int) bool { return s.notes[a].Time < s.notes[b].Time })
	}
	s.mu.Unlock()

	return s.save()
}

// Delete removes the note at a time. Removing one that is not there is not an
// error; the interface and the file may simply have disagreed for a moment.
func (s *Store) Delete(seconds float64) error {
	s.mu.Lock()
	if i := indexAt(s.notes, seconds, matchTolerance); i >= 0 {
		s.notes = append(s.notes[:i], s.notes[i+1:]...)
	}
	s.mu.Unlock()

	return s.save()
}

// ToggleStar flips a note's star.
func (s *Store) ToggleStar(seconds float64) error {
	s.mu.Lock()
	if i := indexAt(s.notes, seconds, matchTolerance); i >= 0 {
		s.notes[i].Starred = !s.notes[i].Starred
	}
	s.mu.Unlock()

	return s.save()
}

// SaveAs rebinds the store to a path the user chose and writes to it.
func (s *Store) SaveAs(path string) error {
	s.mu.Lock()
	s.path = path
	s.origin = OriginChosen
	s.mu.Unlock()

	return s.save()
}

// Reload re-reads the bound file if it has changed on disk, reporting whether
// anything actually changed.
//
// A file that has been deleted is left alone rather than emptying the list: an
// unplugged drive or a sync in progress is not an instruction to discard the
// user's notes.
func (s *Store) Reload() (bool, error) {
	s.mu.RLock()
	path, seen := s.path, s.seen
	s.mu.RUnlock()

	if path == "" {
		return false, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return false, nil
	}

	current := stamp{size: info.Size(), modTime: info.ModTime().UnixNano()}
	if current == seen {
		return false, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("notes: opening %s: %w", path, err)
	}
	defer file.Close()

	parsed, err := Parse(file)
	if err != nil {
		return false, fmt.Errorf("notes: reading %s: %w", path, err)
	}

	s.mu.Lock()
	s.notes = parsed
	s.seen = current
	s.mu.Unlock()

	return true, nil
}

// save writes the notes out, or clears the file when the last one is deleted.
func (s *Store) save() error {
	s.mu.RLock()
	path, videoName := s.path, s.videoName
	data := Format(s.notes, videoName)
	s.mu.RUnlock()

	if path == "" {
		return fmt.Errorf("there is nowhere to save these notes yet")
	}

	if err := appdir.WriteFileAtomic(path, data, 0o644); err != nil {
		s.mu.Lock()
		s.origin = OriginUnsaved
		s.mu.Unlock()
		return fmt.Errorf("notes could not be saved to %s: %w", path, err)
	}

	if info, err := os.Stat(path); err == nil {
		s.mu.Lock()
		s.seen = stamp{size: info.Size(), modTime: info.ModTime().UnixNano()}
		s.mu.Unlock()
	}
	return nil
}

// indexAt finds the note nearest a time within a tolerance, or -1.
func indexAt(notes []Note, seconds, tolerance float64) int {
	best, bestDelta := -1, tolerance
	for i, note := range notes {
		delta := math.Abs(note.Time - seconds)
		if delta <= bestDelta {
			best, bestDelta = i, delta
		}
	}
	return best
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/notes/ -v`
Expected: PASS, every test.

If `TestUnwritableFolderKeepsNotesInMemory` fails because `WriteFileAtomic` creates parent directories, change the test to use a path whose parent is a regular file — as written it already does, since `blocker` is a file and `blocker/lesson.mp4.notes` cannot be created on Windows or Unix.

- [ ] **Step 5: Commit**

```bash
git add internal/notes/
git commit -m "Bind a notes file to the open video and keep it in step"
```

---

## Task 3: Remember a chosen notes file per video

**Files:**
- Modify: `internal/history/history.go`
- Modify: `internal/history/history_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/history/history_test.go`:

```go
func TestSetNotesPathSurvivesRecord(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	video := filepath.Join(dir, "lesson.mp4")
	notes := filepath.Join(dir, "elsewhere.notes")

	if err := store.SetNotesPath(video, notes); err != nil {
		t.Fatalf("SetNotesPath: %v", err)
	}

	// A later position update must not wipe the binding.
	if err := store.Record(video, "Lesson", 600, 3600); err != nil {
		t.Fatalf("Record: %v", err)
	}

	entry, ok := store.Lookup(video)
	if !ok {
		t.Fatal("the entry disappeared")
	}
	if entry.NotesPath != notes {
		t.Errorf("NotesPath = %q, want %q", entry.NotesPath, notes)
	}

	// And it must survive a reload from disk.
	reopened, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	again, ok := reopened.Lookup(video)
	if !ok {
		t.Fatal("the entry did not persist")
	}
	if again.NotesPath != notes {
		t.Errorf("persisted NotesPath = %q, want %q", again.NotesPath, notes)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/history/ -run TestSetNotesPath -v`
Expected: FAIL — `entry.NotesPath undefined`.

- [ ] **Step 3: Add the field**

In `internal/history/history.go`, add to `Entry` after `Duration`:

```go
	// NotesPath is a notes file the user chose for this video, set only when it
	// is not the default location beside the file. Remembering it means the
	// manual re-attach after a read-only folder is a one-time cost.
	NotesPath string `json:"notesPath,omitempty"`
```

- [ ] **Step 4: Add the setter**

In `internal/history/history.go`, after `Forget`:

```go
// SetNotesPath remembers which notes file belongs to a video.
//
// An empty path clears the binding, returning the video to the default location
// beside itself.
func (s *Store) SetNotesPath(path, notesPath string) error {
	if path == "" {
		return nil
	}

	s.mu.Lock()
	k := key(path)
	entry, exists := s.entries[k]
	if !exists {
		entry.Path = path
		entry.Filename = filepath.Base(path)
		entry.UpdatedAt = time.Now()
	}
	entry.NotesPath = notesPath
	s.entries[k] = entry
	s.pruneLocked()
	snapshot := s.snapshotLocked()
	s.mu.Unlock()

	return s.save(snapshot)
}
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/history/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/history/
git commit -m "Remember which notes file belongs to a video"
```

---

## Task 4: The three settings

**Files:**
- Modify: `internal/settings/settings.go`
- Modify: `internal/settings/settings_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/settings/settings_test.go`:

```go
func TestNoteDefaults(t *testing.T) {
	def := Defaults()

	if def.NoteCaptureOffset != 5 {
		t.Errorf("NoteCaptureOffset = %v, want 5", def.NoteCaptureOffset)
	}
	if !def.PauseWhileComposingNote {
		t.Error("PauseWhileComposingNote should default to true")
	}
	if !def.ShowNoteMarks {
		t.Error("ShowNoteMarks should default to true")
	}
}

func TestNoteCaptureOffsetIsClamped(t *testing.T) {
	tests := []struct {
		in   float64
		want float64
	}{
		{0, 0},
		{5, 5},
		{60, 60},
		{-1, 5},
		{600, 5},
	}

	for _, tc := range tests {
		s := Defaults()
		s.NoteCaptureOffset = tc.in
		s.Normalise()
		if s.NoteCaptureOffset != tc.want {
			t.Errorf("offset %v normalised to %v, want %v", tc.in, s.NoteCaptureOffset, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/settings/ -run TestNote -v`
Expected: FAIL — `NoteCaptureOffset undefined`.

- [ ] **Step 3: Add the fields**

In `internal/settings/settings.go`, add to the `Settings` struct near `FollowTranscript`:

```go
	// NoteCaptureOffset is how far before the current position a note is
	// stamped, in seconds. A moment is recognised as worth noting a few seconds
	// after it has passed. Zero disables the adjustment.
	NoteCaptureOffset float64 `json:"noteCaptureOffset"`
	// PauseWhileComposingNote pauses playback while the note composer is open,
	// so the video does not run on while the viewer types.
	PauseWhileComposingNote bool `json:"pauseWhileComposingNote"`
	// ShowNoteMarks draws a tick on the seek bar for each note.
	ShowNoteMarks bool `json:"showNoteMarks"`
```

Add to `Defaults()`:

```go
		NoteCaptureOffset:       5,
		PauseWhileComposingNote: true,
		ShowNoteMarks:           true,
```

Add to `Normalise()`:

```go
	// A negative offset would stamp notes in the future, and anything past a
	// minute is a typo rather than a preference.
	if !(s.NoteCaptureOffset >= 0) || s.NoteCaptureOffset > 60 {
		s.NoteCaptureOffset = def.NoteCaptureOffset
	}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/settings/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/settings/
git commit -m "Add the note capture settings"
```

---

## Task 5: The backend API

**Files:**
- Create: `api_notes.go`
- Modify: `app.go`
- Modify: `api_media.go:169` (the block that sets `currentPath`)

- [ ] **Step 1: Add the store to `App`**

In `app.go`, add to the `App` struct after `playlist *playlist.List`:

```go
	notes    *notes.Store
```

Add `"playerone/internal/notes"` to the imports.

Find where the other stores are constructed (the `NewApp` or `startup` function that builds `settings`, `history` and `playlist`) and add:

```go
	notes: notes.NewStore(),
```

- [ ] **Step 2: Bind the notes file when a video opens**

In `api_media.go`, inside `Open`, extend the block that currently reads:

```go
	a.mu.Lock()
	a.currentPath = abs
	a.mediaInfo = nil
	a.mu.Unlock()
```

to:

```go
	a.mu.Lock()
	a.currentPath = abs
	a.mediaInfo = nil
	a.mu.Unlock()

	a.bindNotes(abs)
```

- [ ] **Step 3: Write `api_notes.go`**

Create `api_notes.go`:

```go
package main

import (
	"fmt"
	"path/filepath"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"playerone/internal/notes"
)

// Timestamped notes.
//
// Notes live in a plain-text file beside the video rather than in a database of
// ours, so they travel with the video, open in any editor and can be handed to
// somebody else. That portability is the whole point of the feature, and it is
// what the fallback below works to preserve when the video's own folder cannot
// be written to.

// NotesResult is what the Notes tab renders.
type NotesResult struct {
	Entries []notes.Note `json:"entries"`

	// Path is the file the notes are read from and written to, empty when there
	// is nowhere to write yet.
	Path string `json:"path"`
	// Origin is "beside", "chosen" or "unsaved".
	Origin string `json:"origin"`
	// Filename is Path's base name, for showing without the full path.
	Filename string `json:"filename"`

	// Status explains an unsaved or unusual state in the user's terms. Empty
	// when everything is ordinary.
	Status string `json:"status"`
}

// notesResult assembles the current state for the frontend.
func (a *App) notesResult(status string) NotesResult {
	entries := a.notes.List()
	if entries == nil {
		entries = []notes.Note{}
	}

	path := a.notes.Path()
	origin := string(a.notes.Origin())

	if status == "" && origin == string(notes.OriginUnsaved) {
		status = "These notes are not saved anywhere yet."
	}

	return NotesResult{
		Entries:  entries,
		Path:     path,
		Origin:   origin,
		Filename: filepath.Base(path),
		Status:   status,
	}
}

// bindNotes attaches the notes store to a newly opened video.
//
// A location the user chose earlier wins over the default beside the video, so
// notes kept elsewhere - because the video sits on a read-only disc, say - come
// back on their own rather than having to be re-attached every session.
func (a *App) bindNotes(videoPath string) {
	if a.notes == nil {
		return
	}

	if a.history != nil {
		if entry, ok := a.history.Lookup(videoPath); ok && entry.NotesPath != "" {
			if err := a.notes.BindTo(entry.NotesPath, filepath.Base(videoPath)); err != nil {
				a.log.Warn("app: could not read the remembered notes file: %v", err)
			}
			a.emitNotes()
			return
		}
	}

	a.notes.Bind(videoPath)
	a.emitNotes()
}

// emitNotes pushes the current notes to the interface.
func (a *App) emitNotes() {
	a.emit("notes:changed", a.notesResult(""))
}

// Notes returns the notes for the open video.
func (a *App) Notes() NotesResult {
	if a.notes == nil {
		return NotesResult{Entries: []notes.Note{}}
	}
	return a.notesResult("")
}

// NoteAt returns the note a capture at this position would edit, so the composer
// opens with what is already there rather than blank.
func (a *App) NoteAt(seconds float64) (notes.Note, error) {
	if a.notes == nil {
		return notes.Note{}, fmt.Errorf("notes are not available")
	}
	note, ok := a.notes.Nearby(seconds)
	if !ok {
		return notes.Note{}, nil
	}
	return note, nil
}

// AddNote files a note, replacing one already within half a second.
//
// When the write fails - a read-only folder, a disconnected share - the note is
// kept and the user is asked where to put it instead. The note is never lost to
// a failed save.
func (a *App) AddNote(seconds float64, text string, starred bool) (NotesResult, error) {
	if a.notes == nil {
		return NotesResult{}, fmt.Errorf("notes are not available")
	}

	if err := a.notes.Add(seconds, text, starred); err != nil {
		a.log.Warn("app: %v", err)
		return a.promptForNotesLocation()
	}

	return a.notesResult(""), nil
}

// DeleteNote removes the note at a time.
func (a *App) DeleteNote(seconds float64) (NotesResult, error) {
	if a.notes == nil {
		return NotesResult{}, fmt.Errorf("notes are not available")
	}
	if err := a.notes.Delete(seconds); err != nil {
		a.log.Warn("app: %v", err)
		return a.notesResult("These notes could not be saved."), nil
	}
	return a.notesResult(""), nil
}

// ToggleNoteStar flips a note's star.
func (a *App) ToggleNoteStar(seconds float64) (NotesResult, error) {
	if a.notes == nil {
		return NotesResult{}, fmt.Errorf("notes are not available")
	}
	if err := a.notes.ToggleStar(seconds); err != nil {
		a.log.Warn("app: %v", err)
		return a.notesResult("These notes could not be saved."), nil
	}
	return a.notesResult(""), nil
}

// promptForNotesLocation asks where to keep notes that could not be written
// beside the video, and remembers the answer.
func (a *App) promptForNotesLocation() (NotesResult, error) {
	const explanation = "These notes could not be saved beside the video - the folder is read-only. " +
		"They are being kept somewhere you choose instead, which means the video and its notes " +
		"are two separate files: move the video and you will need to load its notes again."

	path, err := a.chooseNotesSavePath()
	if err != nil {
		return a.notesResult(explanation), nil
	}
	if path == "" {
		// Cancelled. The notes stay in memory and the panel says so.
		return a.notesResult(explanation), nil
	}

	if err := a.notes.SaveAs(path); err != nil {
		return a.notesResult(fmt.Sprintf("%v", err)), nil
	}
	a.rememberNotesPath(path)

	return a.notesResult(""), nil
}

// SaveNotesAs writes the notes to a location the user picks and binds them there.
func (a *App) SaveNotesAs() (NotesResult, error) {
	if a.notes == nil {
		return NotesResult{}, fmt.Errorf("notes are not available")
	}

	path, err := a.chooseNotesSavePath()
	if err != nil {
		return a.notesResult(""), err
	}
	if path == "" {
		return a.notesResult(""), nil
	}

	if err := a.notes.SaveAs(path); err != nil {
		return a.notesResult(""), fmt.Errorf("%v", err)
	}
	a.rememberNotesPath(path)

	a.log.Info("app: notes saved to %s", path)
	return a.notesResult(""), nil
}

// LoadNotesFrom attaches a notes file the user picks to the open video.
func (a *App) LoadNotesFrom() (NotesResult, error) {
	if a.notes == nil {
		return NotesResult{}, fmt.Errorf("notes are not available")
	}

	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Load notes",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Notes", Pattern: "*" + notes.Extension},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	})
	if err != nil {
		return a.notesResult(""), fmt.Errorf("could not show the file dialog: %v", err)
	}
	if path == "" {
		return a.notesResult(""), nil
	}

	if err := a.notes.BindTo(path, filepath.Base(a.CurrentPath())); err != nil {
		return a.notesResult(""), fmt.Errorf("%v", err)
	}
	a.rememberNotesPath(path)

	a.log.Info("app: notes loaded from %s", path)
	return a.notesResult(""), nil
}

// chooseNotesSavePath shows the save dialog, defaulting to the video's own name.
func (a *App) chooseNotesSavePath() (string, error) {
	name := "notes" + notes.Extension
	if current := a.CurrentPath(); current != "" {
		name = filepath.Base(current) + notes.Extension
	}

	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "Save notes",
		DefaultFilename: name,
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Notes", Pattern: "*" + notes.Extension},
		},
	})
	if err != nil {
		return "", fmt.Errorf("could not show the save dialog: %v", err)
	}
	if path == "" {
		return "", nil
	}

	// The dialog does not always append the extension, and a notes file without
	// one is awkward to find again.
	if !strings.EqualFold(filepath.Ext(path), notes.Extension) {
		path += notes.Extension
	}
	return path, nil
}

// rememberNotesPath records a chosen location against the open video.
func (a *App) rememberNotesPath(path string) {
	current := a.CurrentPath()
	if current == "" || a.history == nil {
		return
	}
	if err := a.history.SetNotesPath(current, path); err != nil {
		a.log.Warn("app: could not remember the notes location: %v", err)
	}
}
```

- [ ] **Step 4: Add `CurrentPath` if it does not exist**

Check with: `grep -n "func (a \*App) CurrentPath" app.go`

If absent, add to `app.go`:

```go
// CurrentPath is the file that is open, empty when nothing is.
func (a *App) CurrentPath() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentPath
}
```

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add api_notes.go app.go api_media.go
git commit -m "Expose notes to the interface"
```

---

## Task 6: Pick up edits made outside the app

**Files:**
- Modify: `app.go`

- [ ] **Step 1: Find the existing ticker**

Run: `grep -n "time.NewTicker\|time.Tick" app.go internal/player/*.go`

The app already polls playback state. Add the notes poll alongside whichever goroutine that runs in; if there is no suitable one, create a dedicated goroutine as below.

- [ ] **Step 2: Add the poll**

In `app.go`, add:

```go
// notesPollInterval is how often the bound notes file is checked for changes
// made outside PlayerOne.
//
// The file is plain text, so it will be edited in Notepad and rewritten by
// OneDrive and Dropbox. Polling rather than watching the filesystem is the
// reliable choice across network shares and sync folders, where change
// notifications are unreliable or absent, and two seconds is far below the
// threshold at which a person notices a delay.
const notesPollInterval = 2 * time.Second

// watchNotesFile re-reads the notes file when it changes on disk.
func (a *App) watchNotesFile(ctx context.Context) {
	ticker := time.NewTicker(notesPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if a.notes == nil {
				continue
			}
			changed, err := a.notes.Reload()
			if err != nil {
				a.log.Warn("app: %v", err)
				continue
			}
			if changed {
				a.emitNotes()
			}
		}
	}
}
```

- [ ] **Step 3: Start it**

In the `startup` function where `a.ctx` is assigned, add:

```go
	go a.watchNotesFile(ctx)
```

- [ ] **Step 4: Build**

Run: `go build ./... && go vet ./...`
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add app.go
git commit -m "Re-read the notes file when it changes on disk"
```

---

## Task 7: Frontend types, state and service calls

**Files:**
- Modify: `frontend/src/types/media.ts`
- Modify: `frontend/src/state/store.ts`
- Modify: `frontend/src/services/player.ts`

- [ ] **Step 1: Regenerate the Wails bindings**

Run: `wails generate module`
Expected: `frontend/wailsjs/go/main/App.d.ts` now contains `AddNote`, `Notes`, `NoteAt`, `DeleteNote`, `ToggleNoteStar`, `SaveNotesAs`, `LoadNotesFrom`.

Verify: `grep -c "AddNote" frontend/wailsjs/go/main/App.d.ts` returns at least 1.

- [ ] **Step 2: Add the types**

In `frontend/src/types/media.ts`:

```ts
/** Where a notes file came from, which is what the panel explains to the user. */
export type NotesOrigin = 'beside' | 'chosen' | 'unsaved';

/** One timestamped note. Empty text is a bookmark. */
export interface Note {
  time: number;
  text: string;
  starred: boolean;
}

export interface NotesResult {
  entries: Note[];
  path: string;
  origin: NotesOrigin;
  filename: string;
  status: string;
}
```

- [ ] **Step 3: Add the state**

In `frontend/src/state/store.ts`:

Change the tab union to the final display order — Chapters, Transcript, Notes,
Playlist, Info (Task 8 reorders the tab strip to match):

```ts
export type PanelTab = 'chapters' | 'transcript' | 'notes' | 'playlist' | 'info';
```

Add to the `AppState` interface, after `transcriptLoading`:

```ts
  notes: NotesResult | null;
  notesQuery: string;
  notesStarredOnly: boolean;

  /** The open note composer, null when it is closed. */
  noteComposer: {
    time: number;
    text: string;
    starred: boolean;
    /** True when this is editing a note that already exists. */
    existing: boolean;
    /** Whether playback was running when the composer opened. */
    wasPlaying: boolean;
  } | null;
```

Add `Note, NotesResult` to the type import at the top of the file.

Add to the initial state object, after `transcriptLoading: false,`:

```ts
    notes: null,
    notesQuery: '',
    notesStarredOnly: false,
    noteComposer: null,
```

Add to `defaultSettings`:

```ts
  noteCaptureOffset: 5,
  pauseWhileComposingNote: true,
  showNoteMarks: true,
```

And add the same three fields to the `Settings` interface in `frontend/src/types/media.ts`:

```ts
  noteCaptureOffset: number;
  pauseWhileComposingNote: boolean;
  showNoteMarks: boolean;
```

- [ ] **Step 4: Add the service calls**

In `frontend/src/services/player.ts`, add a section after the playlist one:

```ts
// --- Notes ---

/** Applies a notes result, surfacing whatever the backend had to say about it. */
function applyNotes(result: NotesResult | undefined): void {
  if (!result) return;
  store.set({ notes: result });
}

export async function refreshNotes(): Promise<void> {
  applyNotes(await guard(() => App.Notes()));
}

export async function addNote(time: number, text: string, starred: boolean): Promise<void> {
  applyNotes(await guard(() => App.AddNote(time, text, starred)));
}

export async function deleteNote(time: number): Promise<void> {
  applyNotes(await guard(() => App.DeleteNote(time)));
}

export async function toggleNoteStar(time: number): Promise<void> {
  applyNotes(await guard(() => App.ToggleNoteStar(time)));
}

export async function saveNotesAs(): Promise<void> {
  applyNotes(await guard(() => App.SaveNotesAs()));
}

export async function loadNotesFrom(): Promise<void> {
  applyNotes(await guard(() => App.LoadNotesFrom()));
}

/**
 * Opens the note composer for the current moment.
 *
 * The stamp is set a few seconds back because a moment is recognised as worth
 * noting only after it has passed. If a note is already there, this edits it
 * rather than filing a second one at almost the same time.
 */
export async function openNoteComposer(): Promise<void> {
  const state = store.get();
  if (!state.playback.fileLoaded) return;

  const offset = state.settings.noteCaptureOffset ?? 0;
  const time = Math.max(0, Math.round(state.playback.position - offset));

  const wasPlaying = !state.playback.paused;
  if (state.settings.pauseWhileComposingNote && wasPlaying) {
    await pause();
  }

  const existing = await guard(() => App.NoteAt(time));

  store.set({
    noteComposer: {
      time,
      text: existing?.text ?? '',
      starred: existing?.starred ?? false,
      existing: Boolean(existing && (existing.text !== '' || existing.starred || existing.time === time)),
      wasPlaying,
    },
  });
}

/** Closes the composer, resuming playback if it was running when it opened. */
export function closeNoteComposer(): void {
  const composer = store.get().noteComposer;
  store.set({ noteComposer: null });

  if (composer?.wasPlaying) void play();
}

export async function commitNoteComposer(text: string, starred: boolean): Promise<void> {
  const composer = store.get().noteComposer;
  if (!composer) return;

  await addNote(composer.time, text, starred);
  closeNoteComposer();
}
```

Add `Note, NotesResult` to the type import block at the top of `player.ts`.

- [ ] **Step 5: Wire the event and the per-file refresh**

In `listen()` in `frontend/src/services/player.ts`, add alongside the other handlers:

```ts
  EventsOn('notes:changed', (result: NotesResult) => {
    store.set({ notes: result });
  });
```

In the `media:opened` handler, reset the notes view state so a new video does not inherit the previous one's search:

```ts
    store.set({ notesQuery: '', notesStarredOnly: false, noteComposer: null });
```

- [ ] **Step 6: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/types/media.ts frontend/src/state/store.ts frontend/src/services/player.ts frontend/wailsjs/
git commit -m "Carry notes through the frontend state"
```

---

## Task 8: The Notes tab

**Files:**
- Create: `frontend/src/components/NotesTab.ts`
- Modify: `frontend/src/components/SidePanel.ts`
- Modify: `frontend/src/styles/panel.css`

- [ ] **Step 1: Write the component**

Create `frontend/src/components/NotesTab.ts`:

```ts
import { store } from '../state/store';
import type { AppState } from '../state/store';
import { deleteNote, loadNotesFrom, openNoteComposer, saveNotesAs, seek, toggleNoteStar } from '../services/player';
import type { Note } from '../types/media';
import { clear, el, findActiveIndex, highlight } from '../util/dom';
import { formatTimePadded } from '../util/time';
import { icon } from './icons';

/**
 * The notes a viewer has taken against this video.
 *
 * Built on the same shape as the transcript: a timestamped list that follows
 * playback and seeks when clicked. A bookmark is a note with no text, so the
 * two are one row type with one of them showing a placeholder instead.
 */
export class NotesTab {
  readonly root: HTMLElement;

  private readonly list: HTMLElement;
  private readonly status: HTMLElement;
  private readonly search: HTMLInputElement;
  private readonly starFilter: HTMLButtonElement;
  private readonly count: HTMLElement;
  private readonly unsaved: HTMLElement;

  private rows: HTMLElement[] = [];
  private times: Array<{ start: number; end: number }> = [];
  private signature = '';
  private activeIndex = -1;

  constructor() {
    this.search = el('input', {
      class: 'panel-search-input',
      type: 'search',
      placeholder: 'Search notes',
      'aria-label': 'Search notes',
    }) as HTMLInputElement;

    this.search.addEventListener('input', () => {
      store.set({ notesQuery: this.search.value });
    });

    this.starFilter = el('button', {
      class: 'follow-button',
      type: 'button',
      title: 'Show only starred notes',
    }) as HTMLButtonElement;
    this.starFilter.append(icon('star'), el('span', {}, 'Starred'));
    this.starFilter.addEventListener('click', () => {
      store.set({ notesStarredOnly: !store.get().notesStarredOnly });
    });

    this.count = el('div', { class: 'notes-count' });
    this.unsaved = el('div', { class: 'notes-unsaved' });
    this.list = el('div', { class: 'transcript-list' });
    this.status = el('div', { class: 'panel-empty' });

    const addButton = el('button', { class: 'notes-action', type: 'button' }, icon('plus'), el('span', {}, 'Add a note  (T)'));
    addButton.addEventListener('click', () => void openNoteComposer());

    const loadButton = el('button', { class: 'notes-action', type: 'button' }, el('span', {}, 'Load notes…'));
    loadButton.addEventListener('click', () => void loadNotesFrom());

    const saveButton = el('button', { class: 'notes-action', type: 'button' }, el('span', {}, 'Save notes as…'));
    saveButton.addEventListener('click', () => void saveNotesAs());

    this.root = el(
      'div',
      { class: 'panel-tab-content' },
      el('div', { class: 'panel-search' }, this.search, this.starFilter),
      this.count,
      this.unsaved,
      this.list,
      this.status,
      el('div', { class: 'notes-actions' }, addButton, loadButton, saveButton),
    );
  }

  mount(): void {
    store.subscribe((state) => this.render(state));
  }

  private render(state: AppState): void {
    this.starFilter.classList.toggle('active', state.notesStarredOnly);
    this.starFilter.setAttribute('aria-pressed', state.notesStarredOnly ? 'true' : 'false');

    const query = state.notesQuery.trim().toLowerCase();
    const entries = state.notes?.entries ?? [];

    const signature = [
      entries.length,
      entries.map((n) => `${n.time}:${n.starred ? 1 : 0}:${n.text.length}`).join(','),
      query,
      state.notesStarredOnly ? 'starred' : 'all',
      state.notes?.status ?? '',
      state.notes?.origin ?? '',
    ].join('|');

    if (signature !== this.signature) {
      this.signature = signature;
      this.build(state, entries, query);
      this.activeIndex = -1;
    }

    this.updateActive(state);
  }

  private build(state: AppState, entries: Note[], query: string): void {
    clear(this.list);
    clear(this.status);
    clear(this.unsaved);
    this.rows = [];
    this.times = [];

    // The unsaved bar is shown rather than a dialog re-asking on every note.
    const status = state.notes?.status ?? '';
    this.unsaved.hidden = status === '';
    if (status !== '') {
      const button = el('button', { class: 'notes-action', type: 'button' }, el('span', {}, 'Save notes as…'));
      button.addEventListener('click', () => void saveNotesAs());
      this.unsaved.append(el('p', {}, status), button);
    }

    if (entries.length === 0) {
      this.count.hidden = true;
      this.status.hidden = false;
      this.status.append(
        el('p', { class: 'panel-empty-title' }, 'No notes yet'),
        el('p', { class: 'panel-empty-detail' },
          'Press T while watching to note the moment you are at. A note with no text is a bookmark.'),
      );
      return;
    }

    const starred = entries.filter((n) => n.starred).length;
    // A bookmark has no text, so it can never match a search. The count is what
    // makes that legible rather than mysterious.
    const matches = entries.filter((note) => {
      if (state.notesStarredOnly && !note.starred) return false;
      if (query !== '' && !note.text.toLowerCase().includes(query)) return false;
      return true;
    });

    this.count.hidden = false;
    this.count.textContent = matches.length === entries.length
      ? `${entries.length} ${entries.length === 1 ? 'note' : 'notes'} · ${starred} starred`
      : `${matches.length} of ${entries.length} notes`;

    this.status.hidden = matches.length > 0;
    if (matches.length === 0) {
      this.status.append(el('p', { class: 'panel-empty-title' }, 'No notes match that.'));
      return;
    }

    const fragment = document.createDocumentFragment();

    for (const note of matches) {
      const star = el('button', {
        class: note.starred ? 'note-star active' : 'note-star',
        type: 'button',
        title: note.starred ? 'Remove the star' : 'Star this note',
      }, icon('star')) as HTMLButtonElement;
      star.addEventListener('click', (event) => {
        event.stopPropagation();
        void toggleNoteStar(note.time);
      });

      const remove = el('button', {
        class: 'note-delete',
        type: 'button',
        title: 'Delete this note',
      }, icon('close')) as HTMLButtonElement;
      remove.addEventListener('click', (event) => {
        event.stopPropagation();
        void deleteNote(note.time);
      });

      const body = note.text === ''
        ? el('span', { class: 'transcript-text note-bookmark' }, 'Bookmark')
        : el('span', { class: 'transcript-text' }, highlight(note.text, query));

      const row = el(
        'div',
        { class: 'transcript-row note-row' },
        el('span', { class: 'transcript-time' }, formatTimePadded(note.time)),
        body,
        star,
        remove,
      );

      row.addEventListener('click', () => {
        void seek(note.time);
      });

      this.rows.push(row);
      this.times.push({ start: note.time, end: Number.MAX_SAFE_INTEGER });
      fragment.append(row);
    }

    // A note runs until the next one starts, which is what makes "the note you
    // are currently in" mean anything as playback moves.
    for (let i = 0; i < this.times.length - 1; i += 1) {
      this.times[i].end = this.times[i + 1].start;
    }

    this.list.append(fragment);
  }

  private updateActive(state: AppState): void {
    if (this.times.length === 0) return;

    const index = findActiveIndex(this.times, state.playback.position);
    if (index === this.activeIndex) return;

    if (this.activeIndex >= 0 && this.rows[this.activeIndex]) {
      this.rows[this.activeIndex].classList.remove('active');
    }
    this.activeIndex = index;
    if (index >= 0) this.rows[index]?.classList.add('active');
  }
}
```

- [ ] **Step 2: Check the icons exist**

Run: `grep -n "star\|plus" frontend/src/components/icons.ts`

If `star` or `plus` are missing, add them to the icon map in `frontend/src/components/icons.ts`, following the shape of the entries already there:

```ts
  star: 'M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z',
  plus: 'M12 5v14M5 12h14',
```

- [ ] **Step 3: Add the tab to the panel, and fix the tab order**

The tabs run **Chapters, Transcript, Notes, Playlist, Info**. Notes belongs
next to Transcript because the two are used the same way — timestamped lists you
read while watching — and Info is reference material you consult once, so it
goes last. This moves `Info` behind `Playlist`, which is a change to the
existing order and is intentional.

In `frontend/src/components/SidePanel.ts`:

Import it: `import { NotesTab } from './NotesTab';`

Add the field, keeping the declarations in display order:

```ts
  private readonly chapters = new ChaptersTab();
  private readonly transcript = new TranscriptTab();
  private readonly notes = new NotesTab();
  private readonly playlist = new PlaylistTab();
  private readonly info = new InfoTab();
```

Replace the tab list array entirely:

```ts
    for (const [key, label] of [
      ['chapters', 'Chapters'],
      ['transcript', 'Transcript'],
      ['notes', 'Notes'],
      ['playlist', 'Playlist'],
      ['info', 'Info'],
    ] as Array<[PanelTab, string]>) {
```

Replace the `panel-body` children so the DOM matches the tab order:

```ts
      el('div', { class: 'panel-body' },
        this.chapters.root,
        this.transcript.root,
        this.notes.root,
        this.playlist.root,
        this.info.root,
      ),
```

Add `this.notes.mount();` in `mount()`, between the transcript and playlist
mounts.

Replace the visibility block in `render()`:

```ts
    // Tabs are hidden rather than unmounted so their scroll position and
    // rendered rows survive switching back and forth.
    this.chapters.root.hidden = state.panelTab !== 'chapters';
    this.transcript.root.hidden = state.panelTab !== 'transcript';
    this.notes.root.hidden = state.panelTab !== 'notes';
    this.playlist.root.hidden = state.panelTab !== 'playlist';
    this.info.root.hidden = state.panelTab !== 'info';
```

Also update the `PanelTab` union in `frontend/src/state/store.ts` to match the
display order, so the type reads the way the interface looks:

```ts
export type PanelTab = 'chapters' | 'transcript' | 'notes' | 'playlist' | 'info';
```

(Task 7 introduced this union; this is its final form.)

- [ ] **Step 4: Add the styles**

Append to `frontend/src/styles/panel.css`:

```css
/* Notes ------------------------------------------------------------------ */

.notes-count {
  padding: 0 12px 8px;
  font-size: 12px;
  color: var(--text-dim);
}

.notes-unsaved {
  margin: 0 12px 8px;
  padding: 10px 12px;
  border-radius: 6px;
  background: var(--warning-bg, rgba(255 176 32 / 12%));
  border: 1px solid var(--warning-border, rgba(255 176 32 / 32%));
  font-size: 12px;
  line-height: 1.5;
}

.notes-unsaved p {
  margin: 0 0 8px;
}

.note-row {
  align-items: flex-start;
  gap: 8px;
}

/* The star and delete buttons stay out of the way until the row is reachable,
   so a long list reads as text rather than as a wall of controls. */
.note-star,
.note-delete {
  flex: none;
  opacity: 0;
  background: none;
  border: 0;
  padding: 2px;
  color: var(--text-dim);
  cursor: pointer;
}

.note-row:hover .note-star,
.note-row:hover .note-delete,
.note-row:focus-within .note-star,
.note-row:focus-within .note-delete,
.note-star.active {
  opacity: 1;
}

.note-star.active {
  color: var(--accent);
}

.note-bookmark {
  color: var(--text-dim);
  font-style: italic;
}

.notes-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: 10px 12px;
  border-top: 1px solid var(--border);
}

.notes-action {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: none;
  color: var(--text);
  font-size: 12px;
  cursor: pointer;
}

.notes-action:hover {
  background: var(--surface-hover);
}
```

Check the variable names against `frontend/src/styles/theme.css` and substitute the ones that actually exist:

Run: `grep -n "^  --" frontend/src/styles/theme.css`

- [ ] **Step 5: Typecheck and build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/NotesTab.ts frontend/src/components/SidePanel.ts frontend/src/components/icons.ts frontend/src/styles/panel.css
git commit -m "Add the Notes panel with search and a starred filter"
```

---

## Task 9: The composer and the T key

**Files:**
- Create: `frontend/src/components/NoteComposer.ts`
- Modify: `frontend/src/main.ts`
- Modify: `frontend/src/keyboard/shortcuts.ts`
- Modify: `frontend/src/styles/controls.css`

- [ ] **Step 1: Write the composer**

Create `frontend/src/components/NoteComposer.ts`:

```ts
import { store } from '../state/store';
import type { AppState } from '../state/store';
import { closeNoteComposer, commitNoteComposer } from '../services/player';
import { el } from '../util/dom';
import { formatTimePadded } from '../util/time';
import { icon } from './icons';

/**
 * The note capture overlay.
 *
 * It opens paused, prefilled when a note is already at this moment, and closes
 * on Enter or Escape. Saving with no text at all files a bookmark, which is the
 * fastest thing the feature can do and deliberately costs nothing extra.
 */
export class NoteComposer {
  readonly root: HTMLElement;

  private readonly time: HTMLElement;
  private readonly textarea: HTMLTextAreaElement;
  private readonly star: HTMLButtonElement;

  private open = false;
  private starred = false;

  constructor() {
    this.time = el('span', { class: 'composer-time' });

    this.textarea = el('textarea', {
      class: 'composer-input',
      rows: '3',
      placeholder: 'Type a note, or save it empty as a bookmark',
      'aria-label': 'Note text',
    }) as HTMLTextAreaElement;

    this.textarea.addEventListener('keydown', (event) => {
      // The textarea is inside the global shortcut guard, so these two keys are
      // handled here rather than in shortcuts.ts.
      if (event.key === 'Enter' && !event.shiftKey) {
        event.preventDefault();
        event.stopPropagation();
        void commitNoteComposer(this.textarea.value, this.starred);
        return;
      }
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        closeNoteComposer();
      }
    });

    this.star = el('button', {
      class: 'composer-star',
      type: 'button',
      title: 'Star this note',
    }, icon('star')) as HTMLButtonElement;
    this.star.addEventListener('click', () => {
      this.starred = !this.starred;
      this.star.classList.toggle('active', this.starred);
      this.textarea.focus();
    });

    const save = el('button', { class: 'composer-save', type: 'button' }, 'Save');
    save.addEventListener('click', () => void commitNoteComposer(this.textarea.value, this.starred));

    const cancel = el('button', { class: 'composer-cancel', type: 'button' }, 'Cancel');
    cancel.addEventListener('click', () => closeNoteComposer());

    this.root = el(
      'div',
      { class: 'note-composer', hidden: '' },
      el('div', { class: 'composer-head' }, this.time, this.star),
      this.textarea,
      el('div', { class: 'composer-actions' },
        el('span', { class: 'composer-hint' }, 'Enter saves · Shift+Enter for a new line · Esc cancels'),
        cancel,
        save,
      ),
    );
  }

  mount(): void {
    store.subscribe((state) => this.render(state));
  }

  private render(state: AppState): void {
    const composer = state.noteComposer;

    if (!composer) {
      this.root.hidden = true;
      this.open = false;
      return;
    }

    // Only fill the field when the composer first opens, or every keystroke
    // would be overwritten by the next store notification.
    if (!this.open) {
      this.open = true;
      this.root.hidden = false;
      this.starred = composer.starred;
      this.textarea.value = composer.text;
      this.star.classList.toggle('active', this.starred);
      this.time.textContent = composer.existing
        ? `Editing the note at ${formatTimePadded(composer.time)}`
        : `Note at ${formatTimePadded(composer.time)}`;
      this.textarea.focus();
      this.textarea.setSelectionRange(this.textarea.value.length, this.textarea.value.length);
    }
  }
}
```

- [ ] **Step 2: Mount it**

In `frontend/src/main.ts`, following how the other overlay components are mounted:

```ts
import { NoteComposer } from './components/NoteComposer';
```

```ts
const noteComposer = new NoteComposer();
```

Append `noteComposer.root` to the same container that holds the player controls, and call `noteComposer.mount();` alongside the other `mount()` calls.

Verify the container name with: `grep -n "\.root" frontend/src/main.ts`

- [ ] **Step 3: Bind the key**

In `frontend/src/keyboard/shortcuts.ts`:

Add `openNoteComposer` to the import from `../services/player`.

Add a case to the bare-key switch, after the `'r'` / `'R'` case:

```ts
    case 't':
    case 'T':
      event.preventDefault();
      void openNoteComposer();
      break;
```

Do **not** touch the `'n'` / `'N'` case: it is next-track, which is why notes use `T`.

- [ ] **Step 4: Add the styles**

Append to `frontend/src/styles/controls.css`:

```css
/* The note composer ------------------------------------------------------ */

.note-composer {
  position: absolute;
  left: 50%;
  bottom: 96px;
  transform: translateX(-50%);
  z-index: 40;
  width: min(560px, calc(100% - 48px));
  padding: 12px;
  border-radius: 10px;
  border: 1px solid var(--border);
  background: var(--surface);
  box-shadow: 0 12px 32px rgb(0 0 0 / 45%);
}

.composer-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  font-size: 12px;
  color: var(--text-dim);
}

.composer-star {
  background: none;
  border: 0;
  padding: 2px;
  color: var(--text-dim);
  cursor: pointer;
}

.composer-star.active {
  color: var(--accent);
}

.composer-input {
  width: 100%;
  resize: vertical;
  padding: 8px;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: var(--surface-sunken, var(--surface));
  color: var(--text);
  font: inherit;
}

.composer-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}

.composer-hint {
  flex: 1;
  font-size: 11px;
  color: var(--text-dim);
}

.composer-save,
.composer-cancel {
  padding: 6px 12px;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: none;
  color: var(--text);
  cursor: pointer;
}

.composer-save {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--accent-text, #fff);
}
```

Substitute variable names that exist in `theme.css` where these guesses do not.

- [ ] **Step 5: Typecheck and build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/NoteComposer.ts frontend/src/main.ts frontend/src/keyboard/shortcuts.ts frontend/src/styles/controls.css
git commit -m "Capture a note with T, paused, a few seconds back"
```

---

## Task 10: Note marks on the timeline

**Files:**
- Modify: `frontend/src/components/Timeline.ts`
- Modify: `frontend/src/styles/controls.css`

- [ ] **Step 1: Add the marks**

In `frontend/src/components/Timeline.ts`, find `chapterSignature` and the method that draws chapter ticks (around line 173).

Add a second field beside it:

```ts
  private noteSignature = '';
```

Add a method modelled on the chapter one:

```ts
  /**
   * Draws a tick for each note.
   *
   * Rebuilt only when the notes actually change, for the same reason the
   * chapter marks are: this runs five times a second.
   */
  private renderNoteMarks(state: AppState, duration: number): void {
    const notes = state.settings.showNoteMarks ? (state.notes?.entries ?? []) : [];

    const signature = `${duration}|${notes.length}|${notes.map((n) => `${n.time}${n.starred ? '*' : ''}`).join(',')}`;
    if (signature === this.noteSignature) return;
    this.noteSignature = signature;

    for (const stale of this.track.querySelectorAll('.timeline-note')) {
      stale.remove();
    }
    if (duration <= 0) return;

    for (const note of notes) {
      if (note.time <= 0 || note.time >= duration) continue;
      const mark = el('span', {
        class: note.starred ? 'timeline-note starred' : 'timeline-note',
        title: note.text || 'Bookmark',
      });
      mark.style.left = `${(note.time / duration) * 100}%`;
      this.track.append(mark);
    }
  }
```

Replace `this.track` with whatever the chapter-mark method appends to — check the existing method and match it exactly.

Call it from the same place the chapter marks are rendered, immediately after that call.

- [ ] **Step 2: Add the styles**

Append to `frontend/src/styles/controls.css`:

```css
.timeline-note {
  position: absolute;
  top: 50%;
  width: 2px;
  height: 10px;
  margin-left: -1px;
  transform: translateY(-50%);
  border-radius: 1px;
  background: var(--text-dim);
  pointer-events: none;
}

.timeline-note.starred {
  height: 14px;
  background: var(--accent);
}
```

- [ ] **Step 3: Typecheck and build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/Timeline.ts frontend/src/styles/controls.css
git commit -m "Mark notes on the seek bar"
```

---

## Task 11: The settings controls

**Files:**
- Modify: the settings drawer component

- [ ] **Step 1: Find the settings UI**

Run: `grep -rn "autoResume\|Auto-resume\|followTranscript" frontend/src/components/Drawer.ts`

That is where the existing toggles are built.

- [ ] **Step 2: Add the three controls**

Following the exact shape of the `autoResume` toggle already there, add:

- A checkbox bound to `settings.pauseWhileComposingNote`, labelled **"Pause while writing a note"**, with the hint *"Otherwise the video runs on while you type."*
- A checkbox bound to `settings.showNoteMarks`, labelled **"Show notes on the seek bar"**.
- A number input bound to `settings.noteCaptureOffset`, `min="0"`, `max="60"`, `step="1"`, labelled **"Stamp notes this many seconds earlier"**, with the hint *"A moment is usually recognised as worth noting just after it passes. 0 uses the exact position."*

Each writes through the same `saveSettings` path the other toggles use.

- [ ] **Step 3: Typecheck and build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/Drawer.ts
git commit -m "Expose the note capture settings"
```

---

## Task 12: Verify end to end, then document

**Files:**
- Modify: `README.md`
- Modify: `docs/architecture.md`

- [ ] **Step 1: Run everything**

```bash
go build ./... && go vet ./... && go test ./...
```

Expected: all packages pass.

```bash
cd frontend && npx tsc --noEmit && npm run build
```

Expected: no errors.

- [ ] **Step 2: Manual verification**

Run the app with `wails dev` and confirm each of these, which the automated tests cannot reach:

1. Open a video, press `T`. Playback pauses, the composer opens, the timestamp is roughly five seconds behind where you were.
2. Type and press Enter. The note appears in the Notes tab, a tick appears on the seek bar, and `<video>.mp4.notes` exists beside the video with the text in it.
3. Press `T` and save with an empty box. A bookmark appears, shown in italics.
4. Click a note. The video seeks there.
5. Star a note, switch the filter to Starred, confirm only it shows. Type in the search box and confirm the count reads `n of m`.
6. Edit the `.notes` file in Notepad, save it, and confirm the panel updates within a few seconds without restarting.
7. Copy a video to a read-only folder (`icacls <folder> /deny "%USERNAME%":(W)`), open it, take a note, and confirm the prompt, the Save dialog, and that the note is not lost when you cancel.
8. Reopen that video and confirm the notes come back from the location you chose.

- [ ] **Step 3: Update the README**

In `README.md`, add to the **The side panel** list, after the Transcript bullet:

```markdown
- **Notes** — press `T` to note the moment you are watching; the video pauses
  while you type and the note is stamped a few seconds back, since you always
  realise afterwards. A note with no text is a bookmark. Searchable, filterable
  to starred only, clickable to jump, and marked on the seek bar. They are saved
  as a plain-text `video.mp4.notes` file beside the video, so they travel with
  it and open in any editor.
```

Add `T` to the keyboard shortcuts table if one exists — check with `grep -n "^| \`" README.md`.

- [ ] **Step 4: Update the architecture doc**

In `docs/architecture.md`, add `internal/notes` to the package list with a one-line description matching the style of the neighbouring entries:

```markdown
- `internal/notes` — the plain-text `.notes` file format, and the store that
  binds one to the open video and keeps it in step with edits made outside the
  app.
```

- [ ] **Step 5: Commit**

```bash
git add README.md docs/architecture.md
git commit -m "Document timestamped notes"
```

---

## Self-review notes

**Spec coverage:** file format → Task 1; store and unsaved state → Task 2; remembered binding → Task 3; settings → Task 4; API, discovery, read-only fallback → Task 5; external edits → Task 6; frontend state → Task 7; tab with search and star filter → Task 8; composer, `T`, offset, pause → Task 9; timeline marks → Task 10; settings UI → Task 11; docs → Task 12.

**Deviations from the spec, both recorded in the spec itself:**

1. The capture key is `T`, not `N` — `N` is already next-track in `shortcuts.ts`.
2. Captured times round to whole seconds, so the in-memory note and the on-disk note are identical and a reload never appears to move one.

**Known soft spots to watch during implementation:**

- The CSS variable names in Tasks 8, 9 and 10 are guesses. Check them against `frontend/src/styles/theme.css` and substitute.
- Task 10 appends to `this.track`; confirm the real field name from the chapter-mark method before copying.
- Task 11 has no code block because it mirrors an existing control whose exact markup helper is only visible in `Drawer.ts`. Copy the `autoResume` toggle's structure exactly.
