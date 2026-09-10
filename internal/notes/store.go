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

// save writes the notes out.
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
