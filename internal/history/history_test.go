package history

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestShouldStore(t *testing.T) {
	cases := []struct {
		name     string
		position float64
		duration float64
		want     bool
	}{
		{"the very start", 0, 3600, false},
		{"still in the first ten seconds", 9.5, 3600, false},
		{"exactly ten seconds in", 10, 3600, true},
		{"well into the video", 1800, 3600, true},
		{"two minutes left is the boundary", 3480, 3600, true},
		{"under two minutes left is effectively finished", 3500, 3600, false},
		{"at the very end", 3600, 3600, false},
		{"duration not known yet", 300, 0, true},
		{"early with no duration", 3, 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldStore(tc.position, tc.duration); got != tc.want {
				t.Errorf("ShouldStore(%v, %v) = %v, want %v", tc.position, tc.duration, got, tc.want)
			}
		})
	}
}

func TestEntryShouldOfferMirrorsShouldStore(t *testing.T) {
	if (Entry{Position: 4200, Duration: 8309}).ShouldOffer() != true {
		t.Error("a mid-video position should be offered")
	}
	if (Entry{Position: 5, Duration: 8309}).ShouldOffer() != false {
		t.Error("a position in the first ten seconds should not be offered")
	}
	if (Entry{Position: 8300, Duration: 8309}).ShouldOffer() != false {
		t.Error("a position at the very end should not be offered")
	}
}

func TestRecordAndLookup(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	const path = `C:\Videos\tutorial1.mkv`
	if err := store.Record(path, "Tutorial One", 4355, 8309); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, ok := store.Lookup(path)
	if !ok {
		t.Fatal("Lookup found nothing after Record")
	}
	if got.Position != 4355 {
		t.Errorf("Position = %v, want 4355", got.Position)
	}
	if got.Duration != 8309 {
		t.Errorf("Duration = %v, want 8309", got.Duration)
	}
	if got.Title != "Tutorial One" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Filename != "tutorial1.mkv" {
		t.Errorf("Filename = %q, want tutorial1.mkv", got.Filename)
	}
}

func TestRecordPersistsAcrossStores(t *testing.T) {
	dir := t.TempDir()

	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Record(`C:\Videos\video2.mp4`, "Video Two", 1082, 5400); err != nil {
		t.Fatalf("Record: %v", err)
	}

	reloaded, err := NewStore(dir)
	if err != nil {
		t.Fatalf("reloading: %v", err)
	}
	got, ok := reloaded.Lookup(`C:\Videos\video2.mp4`)
	if !ok {
		t.Fatal("entry did not survive a reload")
	}
	if got.Position != 1082 {
		t.Errorf("Position = %v, want 1082", got.Position)
	}
}

// Watching to the end must clear the resume point while keeping the file in the
// recent list, so reopening starts from the beginning.
func TestFinishingClearsThePositionButKeepsTheEntry(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	const path = `C:\Videos\finished.mkv`
	if err := store.Record(path, "", 4000, 8309); err != nil {
		t.Fatalf("Record mid-video: %v", err)
	}
	if err := store.Record(path, "", 8305, 8309); err != nil {
		t.Fatalf("Record at the end: %v", err)
	}

	got, ok := store.Lookup(path)
	if !ok {
		t.Fatal("the entry was removed; it should have been kept")
	}
	if got.Position != 0 {
		t.Errorf("Position = %v, want 0 once the video is finished", got.Position)
	}
	if !got.ShouldOffer() == false {
		t.Error("a cleared position must not be offered")
	}
}

func TestPositionsInTheFirstTenSecondsAreNotStored(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	const path = `C:\Videos\barely-started.mkv`
	if err := store.Record(path, "", 4, 8309); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, ok := store.Lookup(path)
	if !ok {
		t.Fatal("the file should still appear in the recent list")
	}
	if got.Position != 0 {
		t.Errorf("Position = %v, want 0", got.Position)
	}
}

// Windows paths are case-insensitive, so the same file must not appear twice.
func TestLookupIsCaseInsensitiveOnPaths(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	if err := store.Record(`C:\Videos\Tutorial.mkv`, "", 500, 8309); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(`c:\videos\tutorial.mkv`, "", 900, 8309); err != nil {
		t.Fatalf("Record with different case: %v", err)
	}

	if n := len(store.Recent(0)); n != 1 {
		t.Fatalf("got %d entries, want 1 - the same file was recorded twice", n)
	}
	got, _ := store.Lookup(`C:\VIDEOS\TUTORIAL.MKV`)
	if got.Position != 900 {
		t.Errorf("Position = %v, want the most recent value 900", got.Position)
	}
}

func TestRecentIsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	for _, name := range []string{"first", "second", "third"} {
		if err := store.Record(filepath.Join(dir, name+".mkv"), "", 600, 8309); err != nil {
			t.Fatalf("Record %s: %v", name, err)
		}
		time.Sleep(2 * time.Millisecond) // keep the timestamps distinct
	}

	recent := store.Recent(0)
	if len(recent) != 3 {
		t.Fatalf("got %d entries, want 3", len(recent))
	}
	if !strings.Contains(recent[0].Filename, "third") {
		t.Errorf("newest entry is %q, want the one recorded last", recent[0].Filename)
	}
	if !strings.Contains(recent[2].Filename, "first") {
		t.Errorf("oldest entry is %q, want the one recorded first", recent[2].Filename)
	}
}

func TestRecentRespectsLimit(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	for i := 0; i < 5; i++ {
		if err := store.Record(filepath.Join(dir, string(rune('a'+i))+".mkv"), "", 600, 8309); err != nil {
			t.Fatalf("Record: %v", err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	if n := len(store.Recent(3)); n != 3 {
		t.Errorf("Recent(3) returned %d entries, want 3", n)
	}
	if n := len(store.Recent(0)); n != 5 {
		t.Errorf("Recent(0) returned %d entries, want all 5", n)
	}
}

func TestRecentMarksMissingFiles(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	present := filepath.Join(dir, "present.mkv")
	if err := os.WriteFile(present, []byte("x"), 0o644); err != nil {
		t.Fatalf("creating the fixture: %v", err)
	}
	missing := filepath.Join(dir, "gone.mkv")

	if err := store.Record(present, "", 600, 8309); err != nil {
		t.Fatalf("Record: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := store.Record(missing, "", 600, 8309); err != nil {
		t.Fatalf("Record: %v", err)
	}

	recent := store.Recent(0)
	byName := map[string]Entry{}
	for _, e := range recent {
		byName[e.Filename] = e
	}

	if !byName["present.mkv"].Exists {
		t.Error("present.mkv is marked as missing")
	}
	if byName["gone.mkv"].Exists {
		t.Error("gone.mkv is marked as existing")
	}
	if len(recent) != 2 {
		t.Errorf("got %d entries, want both kept - a missing file is flagged, not dropped", len(recent))
	}
}

func TestPruneKeepsTheNewestMaxEntries(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	for i := 0; i < MaxEntries+7; i++ {
		p := filepath.Join(dir, "v", string(rune('A'+i%26))+string(rune('0'+i/26))+".mkv")
		if err := store.Record(p, "", 600, 8309); err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
		time.Sleep(time.Millisecond)
	}

	recent := store.Recent(0)
	if len(recent) != MaxEntries {
		t.Fatalf("got %d entries, want the list capped at %d", len(recent), MaxEntries)
	}

	// The most recently recorded file must have survived.
	last := filepath.Join(dir, "v", string(rune('A'+(MaxEntries+6)%26))+string(rune('0'+(MaxEntries+6)/26))+".mkv")
	if _, ok := store.Lookup(last); !ok {
		t.Errorf("the newest entry %q was pruned", last)
	}
}

func TestForgetAndClear(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	a := filepath.Join(dir, "a.mkv")
	b := filepath.Join(dir, "b.mkv")
	if err := store.Record(a, "", 600, 8309); err != nil {
		t.Fatalf("Record a: %v", err)
	}
	if err := store.Record(b, "", 600, 8309); err != nil {
		t.Fatalf("Record b: %v", err)
	}

	if err := store.Forget(a); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if _, ok := store.Lookup(a); ok {
		t.Error("a.mkv is still present after Forget")
	}
	if _, ok := store.Lookup(b); !ok {
		t.Error("Forget removed the wrong entry")
	}

	if err := store.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if n := len(store.Recent(0)); n != 0 {
		t.Errorf("got %d entries after Clear, want 0", n)
	}

	reloaded, err := NewStore(dir)
	if err != nil {
		t.Fatalf("reloading: %v", err)
	}
	if n := len(reloaded.Recent(0)); n != 0 {
		t.Errorf("Clear did not persist: %d entries survived a reload", n)
	}
}

func TestCorruptFileYieldsEmptyHistoryNotAFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("<<not json>>"), 0o644); err != nil {
		t.Fatalf("writing corrupt history: %v", err)
	}

	store, err := NewStore(dir)
	if err == nil {
		t.Error("expected an error describing the corrupt file")
	}
	if store == nil {
		t.Fatal("NewStore returned no store")
	}
	if n := len(store.Recent(0)); n != 0 {
		t.Errorf("got %d entries, want 0", n)
	}

	// The store must still be usable.
	if err := store.Record(filepath.Join(dir, "new.mkv"), "", 600, 8309); err != nil {
		t.Fatalf("Record after a corrupt load: %v", err)
	}
}

func TestRecordIgnoresEmptyPath(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Record("", "", 600, 8309); err != nil {
		t.Fatalf("Record with an empty path: %v", err)
	}
	if n := len(store.Recent(0)); n != 0 {
		t.Errorf("got %d entries, want 0", n)
	}
}

// A title supplied once must not be wiped by a later position-only update.
func TestRecordKeepsAKnownTitle(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	p := filepath.Join(dir, "titled.mkv")
	if err := store.Record(p, "The Real Title", 600, 8309); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(p, "", 900, 8309); err != nil {
		t.Fatalf("Record without a title: %v", err)
	}

	got, _ := store.Lookup(p)
	if got.Title != "The Real Title" {
		t.Errorf("Title = %q, want it preserved", got.Title)
	}
}

func TestConcurrentRecordsDoNotCorruptTheFile(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = store.Record(filepath.Join(dir, string(rune('a'+n%26))+".mkv"), "", 600, 8309)
			_ = store.Recent(5)
		}(i)
	}
	wg.Wait()

	if _, err := NewStore(dir); err != nil {
		t.Fatalf("history file was corrupted by concurrent writes: %v", err)
	}
}
