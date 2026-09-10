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

// readOnlyExplanation is shown when the video's own folder refused the write.
//
// It says outright that the two files have been separated, because the whole
// promise of the feature is that notes travel with the video, and a user who is
// not told that has quietly lost it.
const readOnlyExplanation = "These notes could not be saved beside the video - the folder is read-only. " +
	"They are being kept somewhere you choose instead, which means the video and its notes are two " +
	"separate files: move the video and you will need to load its notes again."

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

	result := NotesResult{
		Entries: entries,
		Path:    path,
		Origin:  origin,
		Status:  status,
	}
	if path != "" {
		result.Filename = filepath.Base(path)
	}
	return result
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
	if a.notes == nil {
		return
	}
	a.emit(eventNotesChanged, a.notesResult(""))
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
//
// A position with no note is not an error: an empty note with a zero time is the
// frontend's signal that it is composing a new one.
func (a *App) NoteAt(seconds float64) (notes.Note, error) {
	if a.notes == nil {
		return notes.Note{}, fmt.Errorf("notes are not available")
	}
	note, ok := a.notes.Nearby(seconds)
	if !ok {
		return notes.Note{Time: -1}, nil
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
		return a.promptForNotesLocation(), nil
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
		return a.notesResult(readOnlyExplanation), nil
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
		return a.notesResult(readOnlyExplanation), nil
	}
	return a.notesResult(""), nil
}

// promptForNotesLocation asks where to keep notes that could not be written
// beside the video, and remembers the answer.
//
// Cancelling leaves the notes in memory with the explanation showing, rather
// than asking again on the next note. Being nagged on every keystroke would be
// worse than a bar that stays put until it is dealt with.
func (a *App) promptForNotesLocation() NotesResult {
	path, err := a.chooseNotesSavePath()
	if err != nil || path == "" {
		return a.notesResult(readOnlyExplanation)
	}

	if err := a.notes.SaveAs(path); err != nil {
		return a.notesResult(fmt.Sprintf("%v", err))
	}
	a.rememberNotesPath(path)

	a.log.Info("app: notes saved to %s", path)
	return a.notesResult("")
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
