package notes

import (
	"os"
	"path/filepath"
	"strings"
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
	if want := "00:12:34 a note"; !strings.Contains(string(raw), want) {
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
