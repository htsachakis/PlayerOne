# PlayerOne — manual test checklist

What the automated tests cannot cover: anything involving a real window, a real
GPU, or a real pointer. Work down the list; each item says what *correct* looks
like, so a wrong result is unambiguous.

Run the build under test:

```
dist\PlayerOne-1.0.0-windows-amd64\PlayerOne.exe
```

If something misbehaves, the log at `%APPDATA%\PlayerOne\playerone.log` has the
detail — including errors from the interface itself. For much more:

```
PLAYERONE_LOG=debug dist\PlayerOne-1.0.0-windows-amd64\PlayerOne.exe
```

**Legend:** ⬜ not tested · ✅ works · ❌ broken (note what you saw)

---

## 1. Startup and opening files

| # | Test | Expected |
|---|---|---|
| 1.1 | ⬜ Launch with no arguments | Window opens, dark theme, "Open a video to begin" |
| 1.2 | ⬜ **File ▸ Open**, choose the Betaflight MKV | Plays immediately, video visible, **pause icon showing** |
| 1.3 | ⬜ Launch with a file path as an argument | That file opens and plays |
| 1.4 | ⬜ Launch with the `testdata/media/course` folder | The three lessons queue, lesson1 plays |
| 1.5 | ⬜ Open a file that is not media (a `.txt`) | Clear red banner, no crash |
| 1.6 | ⬜ Open a file, then delete it and use **Recent** | Entry greyed with strikethrough, "file not found" |
| 1.7 | ⬜ **File ▸ Close** | Video hides, empty state returns |

## 2. Playback

| # | Test | Expected |
|---|---|---|
| 2.1 | ⬜ `Space` | Toggles play/pause; **the icon matches the state** |
| 2.2 | ⬜ `←` / `→` | Jumps back/forward 5 seconds |
| 2.3 | ⬜ `Shift`+`←`/`→` | 30 seconds |
| 2.4 | ⬜ `Ctrl`+`←`/`→` | 60 seconds |
| 2.5 | ⬜ Click part-way along the seek bar | Jumps there; time matches where you clicked |
| 2.6 | ⬜ Drag the seek bar handle | Handle follows the pointer smoothly, does not snap back |
| 2.7 | ⬜ Hover the seek bar | Tooltip shows the time and the chapter name |
| 2.8 | ⬜ `↑` / `↓` | Volume changes |
| 2.9 | ⬜ **Drag the volume slider to the far right (150)** | No error banner — *this used to fail* |
| 2.10 | ⬜ `M` | Mutes and unmutes; icon changes |

## 3. Speed and direction

| # | Test | Expected |
|---|---|---|
| 3.1 | ⬜ `]` repeatedly up to 8x | **Every step works, including 1x, 2x, 3x, 4x, 8x** — *whole numbers used to fail* |
| 3.2 | ⬜ `[` back down to 0.25x | Each step applies |
| 3.3 | ⬜ `0` | Returns to 1x |
| 3.4 | ⬜ Speed menu, pick 2x | Applies; button label reads 2x |
| 3.5 | ⬜ At 2x, listen to speech | Pitch corrected, not chipmunked |
| 3.6 | ⬜ `R` | Plays **backwards**; icon flips to the forward arrows |
| 3.7 | ⬜ `R` again | **Returns to forward** — *this used to be one-way* |
| 3.8 | ⬜ Reverse on the long MKV | May stutter — expected, mpv re-decodes backwards |

## 4. Chapters

| # | Test | Expected |
|---|---|---|
| 4.1 | ⬜ Open the Betaflight MKV, **Chapters** tab | 59 chapters, starting `00:00 Learning PID tuning from the MASTER` |
| 4.2 | ⬜ Click `05:42 Check CPU` | Jumps to exactly 5:42 |
| 4.3 | ⬜ Let playback cross a chapter boundary | Highlight moves to the new chapter |
| 4.4 | ⬜ Search "blackbox" | List filters, matches highlighted |
| 4.5 | ⬜ Clear the search | Full list returns |
| 4.6 | ⬜ Open a file with no chapters (a course lesson) | "No chapters found in this video." |
| 4.7 | ⬜ Look at the seek bar | Small tick marks at each chapter start |

## 5. Transcript

| # | Test | Expected |
|---|---|---|
| 5.1 | ⬜ **Transcript** tab on the Betaflight MKV | ~1473 timestamped lines |
| 5.2 | ⬜ Click a line | Jumps to that moment |
| 5.3 | ⬜ Watch while playing | Active line highlights and scrolls itself into view |
| 5.4 | ⬜ Scroll away manually while playing | Auto-scrolling pauses ~4s, then resumes |
| 5.5 | ⬜ Turn **Follow playback** off | Highlight still moves, view stays put |
| 5.6 | ⬜ Search "PIDToolbox" | Filters to matching lines, terms highlighted |
| 5.7 | ⬜ Switch to the *embedded* English subtitle track | Transcript re-extracts (brief loading state), lines appear |
| 5.8 | ⬜ Turn subtitles **Off** | Panel explains subtitles are off |
| 5.9 | ⬜ Open a video with no subtitles | Panel explains why, does not sit blank |

## 5a. Notes

Notes are written to a plain-text `<video>.notes` file beside the video. Keep
that file open in an editor for these; several tests are about what lands in it.

| # | Test | Expected |
|---|---|---|
| 5a.1 | ⬜ Press `T` while playing | Video **pauses**, composer opens **below the video**, never behind it |
| 5a.2 | ⬜ Look at the stamped time | Roughly 5s behind where you were, rounded to the second |
| 5a.3 | ⬜ Type a note, press `Enter` | Saves, composer closes, playback resumes |
| 5a.4 | ⬜ Look beside the video | `<video-filename>.notes` exists, holding `00:MM:SS your text` |
| 5a.5 | ⬜ Press `T`, press `Enter` with an empty box | A **bookmark**: italic "Bookmark" row, bare timestamp in the file |
| 5a.6 | ⬜ Press `Esc` in the composer | Closes, nothing saved, playback resumes |
| 5a.7 | ⬜ Press `T` while **paused** | Playback stays paused after saving |
| 5a.8 | ⬜ Press `Shift`+`Enter` in the composer | New line, does not save |
| 5a.9 | ⬜ Click a note | Jumps to that moment |
| 5a.10 | ⬜ Watch while playing | The current note highlights as playback passes it |
| 5a.11 | ⬜ Hover a note row | Star and delete appear; they stay hidden otherwise |
| 5a.12 | ⬜ Star a note | Count reads `N notes · 1 starred`; `*` appears in the file |
| 5a.13 | ⬜ Delete a note | Row and its file line both go |
| 5a.14 | ⬜ Search note text | Filters, term highlighted, count reads `n of m notes` |
| 5a.15 | ⬜ Search while bookmarks exist | Bookmarks vanish (they have no text); the count says so |
| 5a.16 | ⬜ **Starred** filter | Only starred notes; combines with the search box |
| 5a.17 | ⬜ Look at the seek bar | A tick per note, starred ones taller and accented |
| 5a.18 | ⬜ Press `T` again within ~half a second of an existing note | **Edits** that note, prefilled, rather than adding a second |
| 5a.19 | ⬜ Edit the `.notes` file in Notepad while playing | Panel updates within ~2s, no restart |
| 5a.20 | ⬜ Hand-write `12:34 note` (`MM:SS`, no indent) and save | Parses; continuation lines attach to the note above |
| 5a.21 | ⬜ Press `N` while the composer is open | Types the letter; **does not** skip to the next track |
| 5a.22 | ⬜ Close the file (**Close**) | Notes tab empties; no note can be filed with nothing open |
| 5a.23 | ⬜ Reopen the video | Notes come back from the sidecar |
| 5a.24 | ⬜ Rename `<video>.mkv.notes` to `<video>.notes` and reopen | Still found |

### 5a.b Read-only folders — *untested, needs a first pass*

Copy a video into a folder you cannot write to (`icacls <folder> /deny
"%USERNAME%:(W)"`), then:

| # | Test | Expected |
|---|---|---|
| 5a.25 | ⬜ Take a note there | Explains the folder is read-only, opens a **Save** dialog defaulting to `<video>.notes` |
| 5a.26 | ⬜ Save somewhere writable | The note you typed is there, not lost |
| 5a.27 | ⬜ Reopen that video later | Notes load from the chosen location, no re-attaching |
| 5a.28 | ⬜ Take a note, **Cancel** the dialog | Note kept in memory; a "Save notes as…" bar stays in the panel |
| 5a.29 | ⬜ Take a second note after cancelling | **No second dialog**; the bar is still the only prompt |
| 5a.30 | ⬜ **Load notes…** and pick a file by hand | Binds it; later notes save there |

### 5a.c Settings

| # | Test | Expected |
|---|---|---|
| 5a.31 | ⬜ Set the capture offset to 0 | `T` stamps the exact position |
| 5a.32 | ⬜ Turn **Pause while writing a note** off | `T` opens the composer, video keeps playing |
| 5a.33 | ⬜ Turn **Show notes on the seek bar** off | Ticks disappear, notes remain |

## 6. Tracks and subtitles

| # | Test | Expected |
|---|---|---|
| 6.1 | ⬜ Open the **CC** menu | Off, plus each track with readable names |
| 6.2 | ⬜ Switch subtitle track while playing | Changes without restarting |
| 6.3 | ⬜ `C` | Toggles subtitles off and back to the previous track |
| 6.4 | ⬜ **Audio** menu | Tracks listed like `English — AAC Stereo` |
| 6.5 | ⬜ **Load subtitle file…**, pick the `.srt` | Attaches, selects, transcript follows |
| 6.6 | ⬜ Subtitle delay −/+ in the CC menu | Subtitles shift; no error banner |
| 6.7 | ⬜ Audio delay −/+ | Applies; no error banner |
| 6.8 | ⬜ **Reset** on a delay | Returns to 0.0s |

## 7. Drag and drop — *fixed, needs testing*

| # | Test | Expected |
|---|---|---|
| 7.1 | ⬜ Drag a video onto the **side panel** | Opens and plays |
| 7.2 | ⬜ Drag a video onto the **control bar / title bar** | Opens and plays |
| 7.3 | ⬜ Drag a video onto the **video area** | Opens and plays (mpv handles this one) |
| 7.4 | ⬜ Drag a video onto the **empty state** | Opens and plays |
| 7.5 | ⬜ Drop a `.srt` while a video plays | Attaches as a subtitle track |
| 7.6 | ⬜ Drop **several** videos at once | Builds a playlist |
| 7.7 | ⬜ Drop a **folder** of videos | Queues its videos in order |
| 7.8 | ⬜ Drop a `.zip` or `.txt` | Friendly banner, nothing breaks |
| 7.9 | ⬜ Drag over the window | Dashed outline and "Drop a video or subtitle file" |

## 8. Playlist, shuffle, repeat

| # | Test | Expected |
|---|---|---|
| 8.1 | ⬜ **Playlist** tab ▸ Add folder ▸ `testdata/media/course` | Three lessons, in order |
| 8.2 | ⬜ Click lesson 3 | Plays it; row marked current with a play glyph |
| 8.3 | ⬜ Let a lesson end | **Advances to the next by itself** |
| 8.4 | ⬜ Let the last lesson end with repeat off | Stops; does not loop |
| 8.5 | ⬜ Repeat ▸ **all**, let the last end | Wraps to the first |
| 8.6 | ⬜ Repeat ▸ **one**, let it end | Replays the same file |
| 8.7 | ⬜ With repeat **one**, press Next | **Still moves on** to the next file |
| 8.8 | ⬜ Shuffle on, press Next repeatedly | Every lesson plays once before any repeats |
| 8.9 | ⬜ Turn shuffle on mid-file | Current file keeps playing, does not jump |
| 8.10 | ⬜ Previous, more than 3s into a file | Restarts the current file |
| 8.11 | ⬜ Previous again, within 3s | Goes to the previous file |
| 8.12 | ⬜ `N` and `B` | Next and previous |
| 8.13 | ⬜ Remove a row with ✕ | Row goes; the playing file keeps playing |
| 8.14 | ⬜ Clear the playlist | Empties; playback continues |
| 8.15 | ⬜ Restart the app | Queue, shuffle and repeat are as you left them |
| 8.16 | ⬜ **Save** in the playlist toolbar | Windows Save dialog opens |
| 8.17 | ⬜ Save without typing an extension | The file gets `.m3u8` |
| 8.18 | ⬜ Clear the playlist, then **Open** and pick that file | The queue returns and starts playing |
| 8.19 | ⬜ Open the saved file in Notepad | Readable `#EXTM3U` with one path per entry |
| 8.20 | ⬜ Open the same file in VLC | It plays there too |
| 8.21 | ⬜ Save, move one of the videos, then open the playlist | The moved entry is struck through, the rest play |
| 8.22 | ⬜ **Save** with an empty playlist | Says there is nothing to save |

## 9. Resume and history

| # | Test | Expected |
|---|---|---|
| 9.1 | ⬜ Watch ~2 min, close, reopen the same file | "Resume from …" with the right time |
| 9.2 | ⬜ Choose **Resume** | Jumps there and plays |
| 9.3 | ⬜ Choose **Start from beginning** | Starts at 0:00 |
| 9.4 | ⬜ Tick **Always resume automatically**, reopen | Resumes with no prompt |
| 9.5 | ⬜ Watch only 5s, close, reopen | **No prompt** — under the 10s threshold |
| 9.6 | ⬜ Seek to within 2 min of the end, close, reopen | **No prompt** — effectively finished |
| 9.7 | ⬜ **Recent** menu | Files newest first, with the time you reached |
| 9.8 | ⬜ Reopen from Recent | Opens with its resume prompt |
| 9.9 | ⬜ ✕ on a recent entry | Removed |

## 10. Window, panel, fullscreen

| # | Test | Expected |
|---|---|---|
| 10.1 | ⬜ `P` or the panel button | Panel hides; **video widens to fill** |
| 10.2 | ⬜ `P` again | Panel returns at the same width |
| 10.3 | ⬜ Drag the divider between video and panel | Panel resizes; video follows live |
| 10.4 | ⬜ `F` | Fullscreen; title bar hidden, **panel still there** |
| 10.5 | ⬜ Leave the mouse still in fullscreen ~3s | Controls hide, video grows to fill |
| 10.6 | ⬜ Move the mouse **over the video** | Controls return within about 1/8 second — *this used to fail; the page gets no mouse events over the native video window* |
| 10.6b | ⬜ Let them hide and wake them several times | Works every time |
| 10.7 | ⬜ `Esc` | Leaves fullscreen |
| 10.8 | ⬜ Double-click the video | Toggles fullscreen |
| 10.9 | ⬜ Resize the window | Video tracks the layout with no black gaps or overlap |
| 10.10 | ⬜ Maximise, close, reopen | Reopens maximised |
| 10.11 | ⬜ Move and resize, close, reopen | Same size and position |
| 10.12 | ⬜ Open a menu (CC / Audio / Speed) | Drawer opens **above** the controls, video shrinks — never covered |

## 11. Info tab

| # | Test | Expected |
|---|---|---|
| 11.1 | ⬜ **Info** on the Betaflight MKV | Container, size, duration, bitrate all populated |
| 11.2 | ⬜ Video section | Codec, profile, 1920×1080, ~29.97 fps, pixel format |
| 11.3 | ⬜ **Decoder** row | `Hardware — d3d11va` (or your GPU's equivalent) |
| 11.4 | ⬜ Audio section | Codec, channels, sample rate |
| 11.5 | ⬜ Track counts | Matches the actual file |
| 11.6 | ⬜ PlayerOne section | Correct paths for mpv, ffmpeg, ffprobe, settings, history |

## 12. Errors and edge cases

| # | Test | Expected |
|---|---|---|
| 12.1 | ⬜ Rename `bin\mpv.exe`, start the app | Explains mpv is missing and lists where it looked. Rename it back |
| 12.2 | ⬜ Rename `bin\ffmpeg.exe`, open a video, Transcript tab | Playback fine; transcript explains ffmpeg is needed. Rename it back |
| 12.3 | ⬜ Open a deliberately corrupt file | Friendly banner; the app stays alive |
| 12.4 | ⬜ Unplug a drive holding a queued file, press Next | Clear message, no crash |
| 12.5 | ⬜ Type in the transcript search, press `Space` | A space is typed; **playback does not toggle** |
| 12.6 | ⬜ Open a file with a Unicode name | Opens; title displays correctly |
| 12.7 | ⬜ Open a file from a path with spaces | Opens |
| 12.8 | ⬜ Leave it playing 10+ minutes | No leak, no drift, position stays accurate |

## 13. Version and updates

| # | Test | Expected |
|---|---|---|
| 13.1 | ⬜ Look at the window title | `PlayerOne v1.1.0` |
| 13.2 | ⬜ **Info ▸ PlayerOne** | Version, commit and build date |
| 13.3 | ⬜ Settings drawer | The version, and a **Check now** button |
| 13.4 | ⬜ **Check now** while on the latest | "PlayerOne x.y.z is the latest version." |
| 13.5 | ⬜ Install an older release, then launch | Update bar appears within ~10s |
| 13.6 | ⬜ **What's new** | Opens the GitHub release page |
| 13.7 | ⬜ **Update now** | Progress bar, a UAC prompt, then PlayerOne closes and the installer runs |
| 13.7b | ⬜ **Update now**, then decline the UAC prompt | Says permission was declined; PlayerOne stays open and playable |
| 13.8 | ⬜ Finish the installer | PlayerOne reopens on the new version |
| 13.9 | ⬜ **Skip this version**, restart | Not offered again |
| 13.9b | ⬜ **Check now** in settings, then **Update now** | Installs; it must not answer "check for updates first" |
| 13.9c | ⬜ Restart twice within a few minutes on an old version | The bar appears every time, not once a day |
| 13.10 | ⬜ Turn update checking off, restart | No check; `settings.json` shows `checkForUpdates: false` |
| 13.11 | ⬜ Run the **portable** copy on an old version | Offers the release page, not an install |
| 13.12 | ⬜ First line of the log | Names the version, commit and build date |

## 14. Open with

| # | Test | Expected |
|---|---|---|
| 14.1 | ⬜ Right-click an `.mkv` ▸ Open with | PlayerOne listed |
| 14.2 | ⬜ Same for `.mp4`, `.avi`, `.mov`, `.mp3` | Listed for all of them |
| 14.3 | ⬜ Check your existing default player | **Unchanged** — PlayerOne must not have taken it |
| 14.4 | ⬜ Settings ▸ Default apps | PlayerOne appears and can be chosen |
| 14.5 | ⬜ Open a file through Open with | It plays |
| 14.6 | ⬜ Uninstall, then right-click a video | PlayerOne is gone from the menu |

## 15. Distribution

| # | Test | Expected |
|---|---|---|
| 15.1 | ⬜ Copy the whole `dist\PlayerOne-1.0.0-windows-amd64` folder elsewhere and run | Works — mpv comes from its own `bin\` |
| 13.2 | ⬜ Run `build\bin\PlayerOne.exe` alone from another folder | Explains mpv is missing (expected: no `bin\` beside it) |
| 13.3 | ⬜ `winget install NSIS.NSIS`, then `pwsh scripts/package.ps1 -Installer` | Installer builds |
| 13.4 | ⬜ Run the installer | Installs, Start menu and desktop shortcuts appear |
| 13.5 | ⬜ Launch the installed copy | Works; its own `bin\` is used |
| 13.6 | ⬜ Uninstall | Removed; `%APPDATA%\PlayerOne` deliberately kept |
| 13.7 | ⬜ Run the release workflow from the Actions tab | Green; both artifacts attached |

---

## Recording results

Note anything broken as: what you did, what happened, what you expected. If the
log has a matching line, include it — errors from the interface land there too:

```
grep -E "ERROR|WARN" %APPDATA%\PlayerOne\playerone.log
```

## Known and accepted

These are deliberate, not bugs:

- **HTML never appears over the video.** The video is a native window; menus
  expand a drawer that shrinks it instead. See `docs/architecture.md` §2.
- **Image-based subtitles (PGS/VobSub) produce no transcript.** They are
  pictures; reading them would need OCR, which is out of scope.
- **Reverse playback is heavy** and may stutter on long or high-bitrate files.
- **A screen capture of the video area may come out black.** The video is
  GPU-composited; that is a capture limitation, not a playback fault.
