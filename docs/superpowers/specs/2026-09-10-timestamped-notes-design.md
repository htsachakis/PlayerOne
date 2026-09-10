# Timestamped notes

Date: 2026-09-10
Status: approved, ready for an implementation plan

## The idea

While watching a tutorial you press a key, the video pauses, you type a
sentence, and it is filed against the moment you were watching. Later the side
panel lists those moments and clicking one takes you back.

A **bookmark** is a note with no text. There is one concept, not two.

Notes live in a plain-text file beside the video, so they travel with it, open
in any editor, and can be mailed to somebody else.

## The file

`lesson-04.mp4.notes`, sitting beside `lesson-04.mp4` - the video's full
filename plus `.notes`, so an `.mp4` and an `.mkv` of the same lesson do not
fight over one file.

```
# PlayerOne notes - lesson-04.mp4

00:00:42
00:12:34 * Closures capture the variable, not the value
00:18:05 The bit about defer running LIFO - rewatch,
         it is the second example that actually shows it
```

The format has few enough rules to hold in your head while hand-editing:

- A line beginning with a timestamp starts a note. `MM:SS`, `H:MM:SS` and
  `HH:MM:SS` all parse, with an optional `.mmm`. PlayerOne always writes
  `HH:MM:SS`.
- A `*` immediately after the timestamp marks the note starred.
- Text after the timestamp is the note's first line. No text is a bookmark.
- Any other non-blank line continues the previous note, indented or not, so a
  hand-edit that forgets the indent still works.
- A blank line inside a note is a paragraph break. Trailing blanks are trimmed.
- Lines beginning with `#` are ignored on read. PlayerOne writes the single
  header line back.
- Lines before the first timestamp are ignored.

Written as UTF-8 without a BOM, CRLF line endings, sorted by timestamp.

**Escaping.** A note whose text genuinely begins with `*` is written `\*` and
read back as a literal `*`. This is the only escape in the format.

**Leniency is the point.** The file will be edited by hand. Anything the parser
cannot classify is treated as note text rather than discarded, and a malformed
file never costs the user the notes it could read.

## Backend

### `internal/notes`

```go
type Note struct {
    Time    float64
    Text    string
    Starred bool
}

func Parse(r io.Reader) ([]Note, error)
func Format(notes []Note, videoName string) []byte
```

Timestamp parsing reuses `internal/timefmt`, extending it if the existing
helpers do not cover every accepted form.

A `Store` owns the loaded notes, the path they are bound to, and whether that
path is writable. It writes atomically (temp file plus rename) as the other
stores in `internal/` do.

### Identity

The file has no note IDs, and inventing them breaks as soon as somebody edits
the file in Notepad. **A note's timestamp is its identity.**

Consequently, pressing the capture key within 0.5s of an existing note opens
that note for editing rather than creating a second one. This removes a class
of synchronisation bugs and behaves well: pressing the key again at a spot you
already marked lets you add to it.

### `api_notes.go`

A new file rather than more of `app.go`, which is already 20KB. Following the
existing `api_*.go` convention:

- `Notes() NotesResult` - the entries, the bound path, and how it was bound:
  `beside` (found or created next to the video), `chosen` (the user picked the
  path, whether through Load notes or after a failed write), or `unsaved` (no
  writable path yet, notes exist only in memory).
- `NoteAt(time float64) (Note, bool)` - the note the composer would edit at this
  position, so the composer opens prefilled rather than blank.
- `AddNote(time float64, text string, starred bool) NotesResult` - creates, or
  replaces the existing note when one lies within 0.5s.
- `DeleteNote(time float64) NotesResult`
- `ToggleNoteStar(time float64) NotesResult`
- `SaveNotesAs() NotesResult` - Save dialog, rebinds, writes.
- `LoadNotesFrom() NotesResult` - Open dialog, rebinds, reads.

The three lookups by time (`NoteAt`, `DeleteNote`, `ToggleNoteStar`) match the
nearest note within 1ms rather than by float equality, since the value makes a
round trip through JSON and the text file.

Every mutation returns the full new list, so the frontend never holds
authoritative state. A mutation also writes the file immediately - except in the
`unsaved` state, where it updates memory and leaves the tab showing its unsaved
bar.

## Binding the file to a video

On opening a video, in order:

1. `lesson.mp4.notes` beside it.
2. `lesson.notes` beside it - what somebody writing one by hand would type.
3. Neither: bind to `lesson.mp4.notes`, but do not create it until there is
   something to save.

**Load notes...** sits beside the existing "load subtitle file" action in the
top bar and binds any file the user picks. That binding is recorded in
`history.json` against the video, so a manual re-attach is a one-time cost
rather than something to repeat every session.

## When the folder cannot be written

Read-only course discs, network shares and locked-down folders are common. The
failure is handled at the moment it actually happens, not pre-emptively:

1. The user presses the key, types, and saves.
2. The write fails.
3. A notice appears: the folder is read-only, choose where to keep these notes.
   It says plainly that the file will not sit beside the video, so moving the
   video means re-attaching.
4. A Save dialog opens, defaulting to `lesson.mp4.notes` in Documents.
5. The chosen path is bound and remembered in `history.json`. Later notes save
   silently.

The note being typed is already in memory, so nothing is lost at any point.

**If the dialog is cancelled**, notes remain in memory and the Notes tab shows a
persistent `Not saved - Save notes as...` bar. PlayerOne does not re-prompt on
every note. Closing the file or quitting with notes in that state prompts once
more.

## External edits

The file is plain text, so it will be edited in Notepad and synchronised by
OneDrive and Dropbox.

PlayerOne polls the bound file's mtime and size every 2 seconds while a video is
open - more reliable than filesystem watchers across network shares and sync
folders, and the app already polls for playback state. When the file changes on
disk it is re-read and the Notes tab refreshes.

Because every note is written the instant it is saved, the only in-memory state
that can collide with a reload is text sitting in the open composer. That text
always wins and is merged in when saved.

## Frontend

### `NotesTab.ts`

Built on `TranscriptTab.ts`, whose interaction this shares almost exactly: a
timestamped list, click to seek, the current entry highlighted as playback
passes it.

Header controls:

- **Search** - filters as you type, matching note text case-insensitively, with
  the matched span highlighted. Clearing restores the full list.
- **Filter** - `All` / `Starred`, defaulting to All, remembered for the session
  so switching tabs does not reset it.
- The two combine: starred-only plus a search term.
- **A count is always visible** - `12 notes, 3 starred`, or `4 of 12` during a
  search - so a filtered list is never mistaken for an empty one.

Bookmarks have no text and therefore cannot match a search. They disappear
while a search is active, and the count line is what makes that legible. This
is accepted rather than worked around.

Follow-playback highlighting tracks the real current note even while the list is
filtered. Each row offers edit and delete.

### Composer

A small overlay above the player controls. The timestamp is shown and can be
nudged by a second either way. Enter saves, Shift+Enter inserts a newline, Esc
cancels.

`shortcuts.ts` already excludes `TEXTAREA` from player keybindings, so typing
does not need new guarding.

### Capture

`N` pauses playback, stamps `position - offset` clamped at zero, and opens the
composer. If a note already exists within 0.5s of that stamp, the composer opens
with its text and star state loaded, and saving replaces it. Playback resumes on
save or cancel **only if it was playing when the key was pressed**.

The offset exists because a moment is recognised as worth noting a few seconds
after it passes.

### Timeline

Note marks reuse the `timeline-mark` machinery that already draws chapter ticks
in `Timeline.ts`, under their own class, rebuilt only when the notes actually
change. Starred notes are drawn distinctly.

## Settings

Three new fields in `internal/settings`, surfaced in the existing settings
panel:

- `noteCaptureOffset` (seconds, default 5, 0 disables)
- `pauseWhileComposingNote` (default true)
- `showNoteMarks` (default true)

## Testing

`internal/notes` gets table-driven tests in the style of `m3u_test.go` and
`history_test.go`:

- Round-trip: format then parse returns the original notes.
- Bookmarks: empty text survives a round trip.
- Stars, including a note whose text begins with a literal `*`.
- Hand-edited input: missing indentation, `MM:SS` timestamps, notes out of
  order, unknown leading lines, a trailing newline or none.
- CRLF and LF input both parse.
- Unicode text and paths.
- Malformed timestamps do not lose the notes around them.

`internal/history` gets tests for storing and returning a manual notes binding.

Manual verification, being filesystem-dependent:

- A video in a read-only folder: the prompt, the Save dialog, the rebind, and
  the persistence of the binding across a restart.
- Editing the `.notes` file in Notepad while the video plays, and confirming the
  tab refreshes without losing composer text.

## Out of scope for v1

- Ranges or clips. A note is a point in time.
- Colours and tags.
- A separate Markdown export - the `.notes` file is already the readable
  document.
- A bookmarks-only filter. `All` / `Starred` stays binary.
- Notes surviving a rename of the video. Moving both files together works; the
  remembered manual binding covers the rest.
