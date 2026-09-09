package playlist

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func paths(dir string, names ...string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, filepath.Join(dir, n))
	}
	return out
}

func newList(t *testing.T) (*List, string) {
	t.Helper()
	dir := t.TempDir()
	l, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return l, dir
}

func names(items []Item) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Filename
	}
	return out
}

func TestReplaceSetsTheQueue(t *testing.T) {
	l, dir := newList(t)

	state := l.Replace(paths(dir, "a.mkv", "b.mkv", "c.mkv"))
	if len(state.Items) != 3 {
		t.Fatalf("got %d items, want 3", len(state.Items))
	}
	if state.Current != 0 {
		t.Errorf("Current = %d, want 0", state.Current)
	}
	if got := names(state.Items); got[0] != "a.mkv" || got[2] != "c.mkv" {
		t.Errorf("order = %v", got)
	}
}

func TestReplaceDropsDuplicates(t *testing.T) {
	l, dir := newList(t)

	state := l.Replace(paths(dir, "a.mkv", "a.mkv", "b.mkv"))
	if len(state.Items) != 2 {
		t.Fatalf("got %d items, want 2: %v", len(state.Items), names(state.Items))
	}
}

func TestAddSkipsItemsAlreadyQueued(t *testing.T) {
	l, dir := newList(t)
	l.Replace(paths(dir, "a.mkv", "b.mkv"))

	state, firstAdded := l.Add(paths(dir, "b.mkv", "c.mkv"))
	if len(state.Items) != 3 {
		t.Fatalf("got %d items, want 3: %v", len(state.Items), names(state.Items))
	}
	if firstAdded != 2 {
		t.Errorf("firstAdded = %d, want 2", firstAdded)
	}

	// Adding only duplicates must report that nothing was added.
	_, firstAdded = l.Add(paths(dir, "a.mkv"))
	if firstAdded != -1 {
		t.Errorf("firstAdded = %d, want -1 when nothing new was added", firstAdded)
	}
}

// Windows paths are case-insensitive; the same file must not queue twice.
func TestAddIsCaseInsensitive(t *testing.T) {
	l, _ := newList(t)
	l.Replace([]string{`C:\Videos\Lesson.mkv`})

	state, _ := l.Add([]string{`c:\videos\lesson.mkv`})
	if len(state.Items) != 1 {
		t.Fatalf("got %d items, want 1: %v", len(state.Items), names(state.Items))
	}
}

func TestNextAdvancesInOrder(t *testing.T) {
	l, dir := newList(t)
	l.Replace(paths(dir, "a.mkv", "b.mkv", "c.mkv"))

	for _, want := range []string{"b.mkv", "c.mkv"} {
		item, _, ok := l.Next(false)
		if !ok {
			t.Fatalf("Next returned nothing, wanted %s", want)
		}
		if item.Filename != want {
			t.Errorf("Next = %s, want %s", item.Filename, want)
		}
	}
}

// With repeat off, reaching the end of the queue by itself must stop.
func TestNextStopsAtTheEndWhenRepeatIsOff(t *testing.T) {
	l, dir := newList(t)
	l.Replace(paths(dir, "a.mkv", "b.mkv"))

	if _, _, ok := l.Next(true); !ok {
		t.Fatal("expected to advance to the second item")
	}
	if _, _, ok := l.Next(true); ok {
		t.Error("expected the queue to finish, but it advanced")
	}
}

// Pressing next at the end still wraps, even with repeat off: an explicit
// action should not silently do nothing.
func TestNextWrapsOnAnExplicitPressEvenWithRepeatOff(t *testing.T) {
	l, dir := newList(t)
	l.Replace(paths(dir, "a.mkv", "b.mkv"))
	l.Next(false)

	item, index, ok := l.Next(false)
	if !ok {
		t.Fatal("pressing next at the end should wrap")
	}
	if item.Filename != "a.mkv" || index != 0 {
		t.Errorf("wrapped to %s (index %d), want a.mkv (0)", item.Filename, index)
	}
}

func TestRepeatAllWrapsAround(t *testing.T) {
	l, dir := newList(t)
	l.Replace(paths(dir, "a.mkv", "b.mkv"))
	l.SetRepeat(RepeatAll)

	l.Next(true) // -> b
	item, _, ok := l.Next(true)
	if !ok {
		t.Fatal("repeat-all should wrap rather than stop")
	}
	if item.Filename != "a.mkv" {
		t.Errorf("wrapped to %s, want a.mkv", item.Filename)
	}
}

func TestRepeatOneReplaysOnlyOnAutoAdvance(t *testing.T) {
	l, dir := newList(t)
	l.Replace(paths(dir, "a.mkv", "b.mkv", "c.mkv"))
	l.SetRepeat(RepeatOne)

	// The file ending replays it.
	item, _, ok := l.Next(true)
	if !ok || item.Filename != "a.mkv" {
		t.Fatalf("auto-advance with repeat-one gave %v (ok=%v), want a.mkv", item.Filename, ok)
	}

	// Pressing next must still move on, or the button looks broken.
	item, _, ok = l.Next(false)
	if !ok || item.Filename != "b.mkv" {
		t.Fatalf("pressing next with repeat-one gave %v (ok=%v), want b.mkv", item.Filename, ok)
	}
}

func TestPreviousWrapsToTheEnd(t *testing.T) {
	l, dir := newList(t)
	l.Replace(paths(dir, "a.mkv", "b.mkv", "c.mkv"))

	item, index, ok := l.Previous()
	if !ok {
		t.Fatal("Previous returned nothing")
	}
	if item.Filename != "c.mkv" || index != 2 {
		t.Errorf("Previous from the first item = %s (%d), want c.mkv (2)", item.Filename, index)
	}
}

// Shuffle must visit every item exactly once before repeating any of them.
func TestShuffleIsAPermutationNotRandomPicks(t *testing.T) {
	l, dir := newList(t)
	l.Replace(paths(dir, "a.mkv", "b.mkv", "c.mkv", "d.mkv", "e.mkv"))
	l.SetShuffle(true)
	l.SetRepeat(RepeatAll)

	seen := map[string]int{}
	current, _, _ := l.Current()
	seen[current.Filename]++

	for i := 0; i < 4; i++ {
		item, _, ok := l.Next(true)
		if !ok {
			t.Fatalf("Next stopped early at step %d", i)
		}
		seen[item.Filename]++
	}

	if len(seen) != 5 {
		t.Fatalf("visited %d distinct items in one pass, want 5: %v", len(seen), seen)
	}
	for name, count := range seen {
		if count != 1 {
			t.Errorf("%s played %d times in one pass, want 1", name, count)
		}
	}
}

// Turning shuffle on must not jump away from what is playing.
func TestShuffleKeepsTheCurrentItem(t *testing.T) {
	l, dir := newList(t)
	l.Replace(paths(dir, "a.mkv", "b.mkv", "c.mkv", "d.mkv"))

	l.Select(2) // c.mkv
	l.SetShuffle(true)

	item, index, ok := l.Current()
	if !ok || item.Filename != "c.mkv" || index != 2 {
		t.Errorf("current after enabling shuffle = %s (%d), want c.mkv (2)", item.Filename, index)
	}
}

func TestSelectPathAddsAnUnqueuedFile(t *testing.T) {
	l, dir := newList(t)
	l.Replace(paths(dir, "a.mkv"))

	state := l.SelectPath(filepath.Join(dir, "z.mkv"))
	if len(state.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(state.Items))
	}
	if state.Current != 1 {
		t.Errorf("Current = %d, want the newly added item at 1", state.Current)
	}
}

func TestSelectPathSelectsAnExistingEntry(t *testing.T) {
	l, dir := newList(t)
	l.Replace(paths(dir, "a.mkv", "b.mkv", "c.mkv"))

	state := l.SelectPath(filepath.Join(dir, "c.mkv"))
	if len(state.Items) != 3 {
		t.Fatalf("got %d items, want 3 - the file was already queued", len(state.Items))
	}
	if state.Current != 2 {
		t.Errorf("Current = %d, want 2", state.Current)
	}
}

func TestRemoveAtKeepsTheCurrentFileSelected(t *testing.T) {
	l, dir := newList(t)
	l.Replace(paths(dir, "a.mkv", "b.mkv", "c.mkv"))
	l.Select(2) // c.mkv

	state := l.RemoveAt(0) // remove a.mkv, which is before the current item
	if state.Current != 1 {
		t.Errorf("Current = %d, want 1 so that c.mkv stays selected", state.Current)
	}
	if item, _, _ := l.Current(); item.Filename != "c.mkv" {
		t.Errorf("current item = %s, want c.mkv", item.Filename)
	}
}

func TestRemoveLastItemClampsCurrent(t *testing.T) {
	l, dir := newList(t)
	l.Replace(paths(dir, "a.mkv", "b.mkv"))
	l.Select(1)

	state := l.RemoveAt(1)
	if state.Current != 0 {
		t.Errorf("Current = %d, want 0", state.Current)
	}

	state = l.RemoveAt(0)
	if state.Current != -1 {
		t.Errorf("Current = %d, want -1 for an empty queue", state.Current)
	}
	if len(state.Items) != 0 {
		t.Errorf("got %d items, want 0", len(state.Items))
	}
}

func TestNextOnAnEmptyQueue(t *testing.T) {
	l, _ := newList(t)

	if _, _, ok := l.Next(true); ok {
		t.Error("Next on an empty queue should return nothing")
	}
	if _, _, ok := l.Previous(); ok {
		t.Error("Previous on an empty queue should return nothing")
	}
	if _, _, ok := l.Current(); ok {
		t.Error("Current on an empty queue should return nothing")
	}
}

func TestStateSurvivesAReload(t *testing.T) {
	dir := t.TempDir()

	l, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.Replace(paths(dir, "a.mkv", "b.mkv", "c.mkv"))
	l.Select(1)
	l.SetShuffle(true)
	l.SetRepeat(RepeatAll)

	reloaded, err := New(dir)
	if err != nil {
		t.Fatalf("reloading: %v", err)
	}

	state := reloaded.State()
	if len(state.Items) != 3 {
		t.Fatalf("got %d items after reload, want 3", len(state.Items))
	}
	if state.Current != 1 {
		t.Errorf("Current = %d, want 1", state.Current)
	}
	if !state.Shuffle {
		t.Error("shuffle did not survive the reload")
	}
	if state.Repeat != RepeatAll {
		t.Errorf("Repeat = %q, want %q", state.Repeat, RepeatAll)
	}
}

func TestCorruptFileYieldsAnEmptyQueue(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("not json"), 0o644); err != nil {
		t.Fatalf("writing the corrupt file: %v", err)
	}

	l, err := New(dir)
	if err == nil {
		t.Error("expected an error describing the corrupt file")
	}
	if l == nil {
		t.Fatal("New returned no playlist")
	}
	if l.Len() != 0 {
		t.Errorf("got %d items, want 0", l.Len())
	}

	// It must still be usable.
	if state := l.Replace(paths(dir, "a.mkv")); len(state.Items) != 1 {
		t.Errorf("the playlist is unusable after a corrupt load")
	}
}

func TestStateMarksMissingFiles(t *testing.T) {
	l, dir := newList(t)

	present := filepath.Join(dir, "present.mkv")
	if err := os.WriteFile(present, []byte("x"), 0o644); err != nil {
		t.Fatalf("creating the fixture: %v", err)
	}
	l.Replace([]string{present, filepath.Join(dir, "gone.mkv")})

	state := l.State()
	if state.Items[0].Missing {
		t.Error("present.mkv is marked missing")
	}
	if !state.Items[1].Missing {
		t.Error("gone.mkv is not marked missing")
	}
}

func TestUpdateCurrentFillsInTitleAndDuration(t *testing.T) {
	l, dir := newList(t)
	l.Replace(paths(dir, "a.mkv", "b.mkv"))

	l.UpdateCurrent("The Real Title", 8309.237)

	item, _, _ := l.Current()
	if item.Title != "The Real Title" {
		t.Errorf("Title = %q", item.Title)
	}
	if item.Duration != 8309.237 {
		t.Errorf("Duration = %v", item.Duration)
	}

	// A later update with no title must not wipe the one already known.
	l.UpdateCurrent("", 8309.237)
	if item, _, _ := l.Current(); item.Title != "The Real Title" {
		t.Errorf("Title was lost: %q", item.Title)
	}
}

func TestParseRepeat(t *testing.T) {
	cases := []struct {
		in   string
		want Repeat
	}{
		{"off", RepeatOff},
		{"all", RepeatAll},
		{"one", RepeatOne},
		{"ALL", RepeatAll},
		{" one ", RepeatOne},
		{"", RepeatOff},
		{"nonsense", RepeatOff},
	}

	for _, tc := range cases {
		if got := ParseRepeat(tc.in); got != tc.want {
			t.Errorf("ParseRepeat(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A folder of numbered lessons must queue in lesson order, not "1, 10, 2".
func TestSortPathsIsNatural(t *testing.T) {
	in := []string{
		`C:\c\lesson10.mkv`,
		`C:\c\lesson2.mkv`,
		`C:\c\lesson1.mkv`,
		`C:\c\lesson20.mkv`,
		`C:\c\lesson3.mkv`,
	}

	got := SortPaths(in)
	want := []string{"lesson1.mkv", "lesson2.mkv", "lesson3.mkv", "lesson10.mkv", "lesson20.mkv"}

	for i, w := range want {
		if filepath.Base(got[i]) != w {
			t.Fatalf("sorted order = %v, want %v", got, want)
		}
	}
}

func TestSortPathsHandlesZeroPadding(t *testing.T) {
	in := []string{`d\ep-010.mp4`, `d\ep-002.mp4`, `d\ep-1.mp4`}
	got := SortPaths(in)

	if filepath.Base(got[0]) != "ep-1.mp4" {
		t.Errorf("first = %s, want ep-1.mp4 (got order %v)", filepath.Base(got[0]), got)
	}
	if filepath.Base(got[2]) != "ep-010.mp4" {
		t.Errorf("last = %s, want ep-010.mp4 (got order %v)", filepath.Base(got[2]), got)
	}
}

func TestConcurrentUseIsSafe(t *testing.T) {
	l, dir := newList(t)
	l.Replace(paths(dir, "a.mkv", "b.mkv", "c.mkv", "d.mkv"))

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			switch n % 5 {
			case 0:
				l.Next(true)
			case 1:
				l.Previous()
			case 2:
				l.SetShuffle(n%2 == 0)
			case 3:
				_ = l.State()
			case 4:
				l.UpdateCurrent("t", float64(n))
			}
		}(i)
	}
	wg.Wait()

	if _, err := New(dir); err != nil {
		t.Fatalf("the playlist file was corrupted by concurrent writes: %v", err)
	}
}
