package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"playerone/internal/playlist"
)

// eventPlaylist carries queue changes to the interface.
const eventPlaylist = "playlist:changed"

// Playlist returns the current queue.
func (a *App) Playlist() playlist.State {
	if a.playlist == nil {
		return playlist.State{Items: []playlist.Item{}, Current: -1, Repeat: playlist.RepeatOff}
	}

	state := a.playlist.State()
	if state.Items == nil {
		state.Items = []playlist.Item{}
	}
	return state
}

func (a *App) emitPlaylist() {
	a.emit(eventPlaylist, a.Playlist())
}

// ChooseFiles shows a dialog that accepts several files at once, for building a
// queue in one step.
func (a *App) ChooseFiles() ([]string, error) {
	paths, err := wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Add videos to the playlist",
		Filters: []wailsruntime.FileFilter{
			{
				DisplayName: "Video files",
				Pattern:     "*.mkv;*.mp4;*.webm;*.mov;*.avi;*.m4v;*.mpg;*.mpeg;*.m2v;*.ts;*.m2ts;*.mts;*.wmv;*.asf;*.flv;*.f4v;*.ogv;*.3gp;*.3g2;*.vob;*.divx;*.rmvb;*.mxf",
			},
			{DisplayName: "Audio files", Pattern: "*.mp3;*.m4a;*.m4b;*.flac;*.opus;*.wav;*.aac;*.ogg;*.oga;*.wma;*.mka;*.ape;*.aiff"},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("could not show the file dialog: %w", err)
	}
	if paths == nil {
		return []string{}, nil
	}
	return paths, nil
}

// ChooseFolder shows a folder picker, for queueing a whole course at once.
func (a *App) ChooseFolder() (string, error) {
	dir, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Add a folder to the playlist",
	})
	if err != nil {
		return "", fmt.Errorf("could not show the folder dialog: %w", err)
	}
	return dir, nil
}

// AddToPlaylist queues files, and folders by scanning them for playable media.
//
// The first newly added item starts playing when nothing is playing already, so
// adding files to an idle player does the obvious thing.
func (a *App) AddToPlaylist(paths []string) (playlist.State, error) {
	expanded, err := a.expandPaths(paths)
	if err != nil {
		return a.Playlist(), err
	}
	if len(expanded) == 0 {
		return a.Playlist(), fmt.Errorf("none of the selected items are media files PlayerOne can play")
	}

	wasEmpty := a.playlist.Len() == 0
	state, firstAdded := a.playlist.Add(expanded)
	a.emitPlaylist()

	if firstAdded < 0 {
		return state, nil
	}

	// Start playing only when there is nothing to interrupt.
	playing := false
	if engine := a.currentEngine(); engine != nil {
		playing = engine.State().FileLoaded
	}
	if wasEmpty || !playing {
		if item, ok := a.playlist.Select(firstAdded); ok {
			a.emitPlaylist()
			if _, err := a.Open(item.Path); err != nil {
				return a.Playlist(), err
			}
			_ = a.Play()
		}
	}

	return a.Playlist(), nil
}

// ReplacePlaylist discards the queue and replaces it, starting the first item.
func (a *App) ReplacePlaylist(paths []string) (playlist.State, error) {
	expanded, err := a.expandPaths(paths)
	if err != nil {
		return a.Playlist(), err
	}
	if len(expanded) == 0 {
		return a.Playlist(), fmt.Errorf("none of the selected items are media files PlayerOne can play")
	}

	a.playlist.Replace(expanded)
	a.emitPlaylist()

	if item, _, ok := a.playlist.Current(); ok {
		if _, err := a.Open(item.Path); err != nil {
			return a.Playlist(), err
		}
		_ = a.Play()
	}
	return a.Playlist(), nil
}

// PlayPlaylistItem plays the queue entry at an index.
func (a *App) PlayPlaylistItem(index int) (playlist.State, error) {
	item, ok := a.playlist.Select(index)
	if !ok {
		return a.Playlist(), fmt.Errorf("that playlist entry no longer exists")
	}
	a.emitPlaylist()

	if _, err := a.Open(item.Path); err != nil {
		return a.Playlist(), err
	}
	_ = a.Play()
	return a.Playlist(), nil
}

// RemoveFromPlaylist drops one entry.
func (a *App) RemoveFromPlaylist(index int) playlist.State {
	state := a.playlist.RemoveAt(index)
	a.emitPlaylist()
	return state
}

// ClearPlaylist empties the queue. Playback is left alone: clearing the list of
// what comes next should not stop what is playing now.
func (a *App) ClearPlaylist() playlist.State {
	state := a.playlist.Clear()
	a.emitPlaylist()
	return state
}

// NextTrack plays the next entry.
func (a *App) NextTrack() error {
	return a.advance(false)
}

// PreviousTrack plays the previous entry.
//
// It restarts the current file when playback is more than a few seconds in,
// which is what every other player does and what a listener expects.
func (a *App) PreviousTrack() error {
	if engine := a.currentEngine(); engine != nil {
		state := engine.State()
		if state.FileLoaded && state.Position > restartThreshold {
			return engine.Seek(a.ctx, 0)
		}
	}

	item, _, ok := a.playlist.Previous()
	if !ok {
		return fmt.Errorf("there is nothing before this in the playlist")
	}
	a.emitPlaylist()

	if _, err := a.Open(item.Path); err != nil {
		return err
	}
	return a.Play()
}

// restartThreshold is how far into a file the previous button restarts it
// rather than moving back a track.
const restartThreshold = 3.0

// advance moves to the next entry.
//
// auto distinguishes a file ending by itself from the user pressing next, which
// is what makes repeat-one replay on its own but still step forward on a press.
func (a *App) advance(auto bool) error {
	if a.playlist == nil {
		return nil
	}

	a.log.Debug("app: advancing the playlist (auto=%v)", auto)

	item, _, ok := a.playlist.Next(auto)
	if !ok {
		if auto {
			a.log.Info("app: the playlist has finished")
			return nil
		}
		return fmt.Errorf("there is nothing after this in the playlist")
	}
	a.emitPlaylist()

	if _, err := a.Open(item.Path); err != nil {
		return err
	}
	return a.Play()
}

// SetShuffle turns shuffled playback on or off.
func (a *App) SetShuffle(on bool) playlist.State {
	state := a.playlist.SetShuffle(on)
	a.log.Info("app: shuffle is %s", onOff(on))
	a.emitPlaylist()
	return state
}

// SetRepeat sets the repeat mode: "off", "all" or "one".
func (a *App) SetRepeat(mode string) playlist.State {
	state := a.playlist.SetRepeat(playlist.ParseRepeat(mode))
	a.log.Info("app: repeat is %s", state.Repeat)
	a.emitPlaylist()
	return state
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// expandPaths turns a mixed selection of files and folders into a list of
// playable files, ordered the way a file manager would show them.
func (a *App) expandPaths(paths []string) ([]string, error) {
	var out []string

	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		info, err := os.Stat(p)
		if err != nil {
			a.log.Warn("app: skipping %s: %v", p, err)
			continue
		}

		if !info.IsDir() {
			if looksLikeMediaFile(p) {
				out = append(out, p)
			}
			continue
		}

		found, err := a.scanFolder(p)
		if err != nil {
			return nil, err
		}
		out = append(out, found...)
	}

	return playlist.SortPaths(out), nil
}

// maxFolderEntries caps a folder scan.
//
// A course folder holds tens of files; a number far beyond that means the user
// picked their whole drive by mistake, and queueing it would be useless as well
// as slow.
const maxFolderEntries = 2000

// scanFolder lists the playable files directly inside a folder.
//
// The scan is deliberately shallow. Recursing would sweep up unrelated media
// from sibling projects, and a course is normally one folder of lessons.
func (a *App) scanFolder(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("%s could not be read: %v", filepath.Base(dir), err)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		full := filepath.Join(dir, entry.Name())
		if looksLikeMediaFile(full) {
			out = append(out, full)
		}
		if len(out) >= maxFolderEntries {
			a.log.Warn("app: %s holds more than %d media files; the rest were skipped", dir, maxFolderEntries)
			break
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("%s contains no media files PlayerOne can play", filepath.Base(dir))
	}
	return out, nil
}
