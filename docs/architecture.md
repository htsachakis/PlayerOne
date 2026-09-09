# PlayerOne — Architecture

> Status: implemented (V1). Validated against a real 2h18m / 59-chapter MKV fixture.

## 1. The central question: how does mpv video get inside a Wails window?

Wails v2 on Windows renders the UI with **WebView2**, which lives in its own child HWND inside
the main window. mpv needs somewhere to draw hardware-accelerated video. There is no supported
way to composite mpv's GPU output *into* a WebView2 surface, so the real decision is **which
native surface mpv draws into, and how we talk to mpv**.

### Options considered

| # | Approach | Verdict |
|---|----------|---------|
| A | `mpv.exe` child process, video into a native child HWND via `--wid`, control via **JSON IPC** over a Windows named pipe | **Chosen** |
| B | `libmpv` (`mpv-2.dll`) via cgo, still embedding with `--wid` | Rejected for V1 |
| C | `libmpv` render API drawing into a texture composited by WebView2 | Rejected — not possible |
| D | HTML `<video>` element | Rejected — defeats the purpose |

### Why A

* **No cgo.** Pure Go build. Nothing but the Go toolchain is needed — no MinGW, no
  `mpv-2.dll` header/ABI matching, no `CGO_ENABLED=1` complications.
* **Crash isolation.** A malformed file that kills mpv kills a *child process*, not the app.
  We detect the exit, surface a friendly error, and respawn. With libmpv, an mpv-side crash
  takes the whole application down — which would violate the hard requirement "never crash
  simply because a media file is malformed".
* **Swappable engine.** Dropping a newer `mpv.exe` into `bin/` upgrades the entire decoder
  stack. No rebuild, no ABI concerns.
* **IPC is mpv's most stable public API.** `--input-ipc-server` and the command/property/event
  protocol have been stable for years and are fully documented, unlike the C API which demands
  careful lifetime and threading discipline.
* **Latency is a non-issue.** Property observation is push-based over the pipe; a command
  round-trip on a local named pipe is well under a millisecond — far below one frame. The UI
  never blocks on it anyway (§5).

### Why not B

libmpv would buy synchronous property reads and slightly richer events. Neither justifies
shipping a DLL, requiring a cgo toolchain, and losing crash isolation. The video embedding
mechanism (`--wid`) — and therefore the airspace limitation below — is *identical* either way,
so B costs real reliability and buys nothing architecturally.

### Why not C

`mpv_render_context` can render into a caller-supplied OpenGL FBO or D3D11 texture, but
WebView2 exposes no public API to accept an external texture for compositing into the page.
The only path would be reading frames back to the CPU and pushing bitmaps into the page —
discarding hardware decoding and adding a full copy per frame. That performs unacceptably at
1080p, let alone 4K.

## 2. The airspace constraint (and why the UI is laid out the way it is)

The video HWND is a **native child window, sibling to the WebView2 HWND**. Windows composites
child HWNDs above WebView2 content in that rectangle. The consequence:

> **HTML cannot be drawn on top of the video.**

This is inherent to `--wid` embedding and would be equally true with libmpv. The UI is
therefore designed so that nothing *needs* to float over video:

* Player controls sit in a bar **below** the video (matching the target design).
* The side panel sits **beside** the video.
* Fullscreen and control auto-hide work by **shrinking or growing the video rectangle**, never
  by overlaying. Hiding the controls in fullscreen resizes the video HWND to the full client
  area; showing them shrinks it by the control-bar height. One `SetWindowPos`, no flicker.
* Transient UI — the resume prompt, error banners, track menus, loading and empty states — is
  placed **outside** the video rectangle, or shown while the video window is hidden.

`internal/winvideo` hides the video HWND entirely whenever no file is loaded, so the HTML empty
state is fully visible.

## 3. Process and thread model

```
                    PlayerOne.exe  (Go / Wails)
  +------------------------------------------------------------+
  |  main HWND  (class "wailsWindow")                          |
  |   +-- WebView2 HWND       <- HTML/TS UI, whole client area |
  |   +-- video HWND          <- class "PlayerOneVideoHost",   |
  |        (created by us)       sized to the video slot,      |
  |                              handed to mpv as --wid        |
  +------------------------------------------------------------+
             | spawn (os/exec, argv array - never a shell string)
             v
   mpv.exe --idle=yes --wid=<HWND> --input-ipc-server=\\.\pipe\playerone-<pid>
             ^
             | newline-delimited JSON over a Windows named pipe (Microsoft/go-winio)
             v
      internal/mpvipc.Client
        - writer: mutex-guarded, one JSON object per line
        - reader goroutine: demuxes replies (by request_id) from events (by "event")
        - pending map[int]chan reply; every request is context-bounded
```

mpv is spawned **once** at startup with `--idle=yes` and stays alive for the life of the app.
Opening a file is a `loadfile` IPC command, so the video HWND is never re-parented and
switching files costs no flicker and no re-embedding.

## 4. Events, not polling

`internal/player` registers mpv property observers at connect time:

`time-pos`, `duration`, `pause`, `volume`, `mute`, `speed`, `play-dir`, `chapter`,
`chapter-list`, `track-list`, `media-title`, `path`, `eof-reached`, `idle-active`, `seeking`,
`sid`, `aid`, `sub-delay`, `audio-delay`, `hwdec-current`.

Observers alone are not sufficient, because mpv reports a property only when it *changes*; see
§12 for the class of bug that follows and how `syncFromMPV` closes it.

mpv pushes `time-pos` at roughly the video frame rate. Forwarding that to the WebView would be
wasteful, so `internal/player/events.go` keeps only the newest value and flushes state to the
frontend on a **5 Hz ticker** (200 ms) — inside the 4–10 updates/sec target. Structural changes
(`track-list`, `chapter-list`, file loaded, EOF, errors) bypass the ticker and emit immediately,
because they are rare and the UI must react at once.

Nothing is polled in a loop.

## 5. Concurrency rules

* All mutable player state lives behind one `sync.RWMutex` in `player.MPVPlayer`.
* The IPC reader goroutine never calls into Wails. It updates state and marks it dirty; the
  5 Hz emitter goroutine is the only thing that calls `runtime.EventsEmit`.
* Every goroutine is owned by a `context.Context` cancelled in `Close()`, and `Close` waits on
  a `sync.WaitGroup`, so shutdown is deterministic and the mpv process is always reaped.
* Frontend-initiated commands are fire-and-forget with a short timeout. The UI never blocks on
  an mpv round trip; the backend stays authoritative and the UI reconciles from the next state
  event.
* `go test -race ./...` passes.

## 6. Transcript pipeline

The transcript is the headline feature, and mpv deliberately does not expose "give me every
subtitle line". The pipeline resolves the current subtitle track to plain timed text:

```
selected subtitle track (mpv track-list entry)
   |
   +-- external file (.srt/.vtt/.ass/.ssa) --> read the file from disk
   |
   +-- embedded track (has ff-index)       --> ffmpeg -i <file> -map 0:<ff-index>
   |                                                  -f srt pipe:1
   v
internal/transcript --> sniff format (SRT vs WebVTT) --> []Entry{Start, End, Text}
                    --> strip presentation tags, preserve punctuation
```

Design notes:

* `ff-index` from mpv's `track-list` *is* the ffmpeg stream index, so mpv track IDs map onto
  `-map 0:N` without guessing.
* **Extraction streams to stdout** (`pipe:1`), so the common path creates no temporary files —
  nothing to sanitize, nothing to clean up.
* **Image-based tracks** (PGS, VobSub, DVB) cannot become text without OCR, which the brief
  rules out. They are detected up front and the UI says plainly that the track is image-based,
  rather than showing a silently empty panel.
* ASS/SSA is converted by ffmpeg to SRT, which preserves timing exactly. The tag stripper then
  removes `{\...}` override blocks and `<i>`/`<b>`/`<u>`/`<font>` markup.
* Extraction is cancellable and runs off the UI path; the panel shows a loading state. Starting
  a new extraction cancels any in-flight one, so rapid track switching cannot pile up work.

## 7. Persistence

`%APPDATA%\PlayerOne\` (via `os.UserConfigDir()` — never beside the executable):

* `settings.json` — volume, mute, speed, auto-resume, follow-transcript, panel visibility and
  width, window geometry, last subtitle/audio language.
* `history.json` — up to 20 recent files: path, title, duration, last position, timestamp.

Writes are **atomic** (temp file in the same directory, then `os.Rename`), so a crash mid-write
cannot corrupt the file. A corrupt or unreadable file falls back to defaults instead of failing
startup. Resume positions are written at most every 10 s while playing, plus on pause, on file
change, and on shutdown. Positions inside the first 10 s, or with under 2 minutes remaining, are
not stored — the video is effectively unstarted or finished.

## 8. External tool resolution

`internal/tools` resolves `mpv`, `ffmpeg` and `ffprobe` in this order:

1. `<dir of executable>/bin/` — plus the `bin/` beside the working directory, which is what
   `wails dev` needs
2. `PATH`, via `exec.LookPath`

No absolute developer paths anywhere. Only `mpv` is required; ffmpeg and ffprobe are optional,
and their absence degrades gracefully (no transcript from *embedded* tracks, less detail in the
Info tab) without ever blocking playback.

All invocation uses `exec.Command` argv arrays, so paths containing spaces and Unicode need no
quoting and never touch a shell. The test fixture's filename contains U+29F8 (`⧸`) specifically
to keep that honest.

## 9. Package layout

| Package | Responsibility |
|---|---|
| `internal/branding` | Single source of the app name — renaming is a one-constant change |
| `internal/logging` | Leveled logger (DEBUG/INFO/WARN/ERROR), file + stderr |
| `internal/timefmt` | The one timestamp formatter (`5:42` / `1:05:42`) |
| `internal/tools` | mpv/ffmpeg/ffprobe discovery |
| `internal/mpvipc` | Named-pipe JSON IPC transport. Knows nothing about players |
| `internal/player` | `Player` interface, models, mpv-backed implementation, event pump |
| `internal/winvideo` | Win32 child window host for `--wid` |
| `internal/media` | ffprobe enrichment, subtitle extraction |
| `internal/transcript` | SRT/WebVTT parsing and tag cleaning |
| `internal/settings` | Settings load/save |
| `internal/history` | Recent files and resume positions |
| `internal/playlist` | The queue, and the shuffle and repeat ordering rules |
| `app.go` | Wails-bound API — the only place that knows about both the UI and the player |

mpv-specific details (property names, IPC message shapes) do not leak past `internal/player`.

## 10. Frontend state

The frontend has exactly one mutable playback state, in `src/state/store.ts`. Components
subscribe to it and never hold their own copy of position, paused, speed, or selected tracks.
Backend events are the only writer of playback fields; user actions call the backend and let
the resulting event update the store. This makes divergence between components structurally
impossible rather than merely discouraged.

## 11. Detecting the end of a file

`--keep-open=yes` holds the last frame on screen instead of unloading, which is
what stops the window flashing black at the end of a lecture. It also means mpv
does **not** emit an `end-file` event at a natural end, so the queue advances on
the `eof-reached` property turning true instead, and only on the transition into
that state.

The `end-file` event is still logged, because the reasons it does carry —
"stop", "redirect" — mean the file was replaced. Advancing on those would load
the next file, which stops the current one, which advances again, without end.

## 12. Two failure modes worth remembering

Both were found in testing and are the sort of thing that reappears if the
reasoning behind the fix is lost.

**A dropped first measurement.** The interface reports its video rectangle as
soon as the page lays out, which is before mpv exists. Discarding that report
left the video window at its initial size for the whole session — the rectangle
never changed again, so nothing ever re-sent it, and the result was sound with
no picture. The rectangle is now recorded whether or not there is a window to
apply it to, and applied when the window appears.

**Assumed state that no event corrects.** mpv emits a property change only when
a value actually changes. Setting `Paused: true` when loading a file therefore
stuck forever: mpv started the file playing, its `pause` property never changed,
and no event ever put the state right. `syncFromMPV` now reads the real values
back once the file is open. Any property whose correct value might already be in
place when the file loads needs that treatment.

## 13. Known limitations

* **Windows only.** `internal/winvideo` is `//go:build windows`. Everything else is portable; a
  Linux/macOS host window would be the only new work.
* **No HTML over video** (§2) — a deliberate, documented trade.
* **Image-based subtitles produce no transcript** (§6) — OCR is explicitly out of scope.
* **The transcript follows the *selected* subtitle track.** Switching tracks re-extracts.
* The video HWND never takes keyboard focus, and mpv's own key handling is disabled
  (`--no-input-default-bindings`, `--input-vo-keyboard=no`), so every shortcut is handled once,
  in the frontend, and cannot double-fire.

## 14. Prepared for, but not in V1

`internal/history` keys entries by absolute path — the same key a
`Bookmark{ID, MediaPath, Timestamp, Title, Note, CreatedAt}` store would use. Adding bookmarks
means a new `internal/bookmarks` package and a fourth side-panel tab. No existing type or
interface has to change.
