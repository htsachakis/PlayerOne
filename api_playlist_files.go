package main

import (
	"fmt"
	"path/filepath"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"playerone/internal/playlist"
)

// Saving and loading playlists as files.
//
// M3U8 rather than a format of our own: a saved playlist then opens in VLC, mpv
// and everything else, and can be copied to another machine or handed to
// someone. The user chooses where it goes, so a playlist can live beside its
// media, in a documents folder, or on a memory stick.

// ExportPlaylist writes the current queue to a file the user chooses.
//
// The returned path is empty when the dialog was cancelled, which is not an
// error.
func (a *App) ExportPlaylist() (string, error) {
	items := a.playlist.State().Items
	if len(items) == 0 {
		return "", fmt.Errorf("there is nothing in the playlist to save")
	}

	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "Save the playlist",
		DefaultFilename: "playlist" + playlist.Extension,
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Playlists", Pattern: "*.m3u8;*.m3u"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("could not show the save dialog: %v", err)
	}
	if path == "" {
		return "", nil
	}

	// The dialog does not always append the extension, and a playlist without
	// one is awkward to find and open again.
	if ext := strings.ToLower(filepath.Ext(path)); ext != ".m3u8" && ext != ".m3u" {
		path += playlist.Extension
	}

	if err := playlist.WriteM3U(path, items); err != nil {
		return "", fmt.Errorf("%v", err)
	}

	a.log.Info("app: saved %d items to %s", len(items), path)
	return path, nil
}

// ImportPlaylist opens a playlist file the user chooses and starts playing it.
func (a *App) ImportPlaylist() (playlist.State, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Open a playlist",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Playlists", Pattern: "*.m3u8;*.m3u"},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	})
	if err != nil {
		return a.Playlist(), fmt.Errorf("could not show the file dialog: %v", err)
	}
	if path == "" {
		return a.Playlist(), nil
	}

	items, err := playlist.ReadM3U(path)
	if err != nil {
		return a.Playlist(), fmt.Errorf("%v", err)
	}

	return a.applyLoadedPlaylist(items, filepath.Base(path))
}

// applyLoadedPlaylist puts loaded entries into the queue and starts playing.
//
// Missing files are kept rather than dropped: a playlist saved months ago may
// point at a drive that is merely unplugged, and silently shortening it would
// destroy the record of what was in it. They are shown struck through instead.
func (a *App) applyLoadedPlaylist(items []playlist.Item, name string) (playlist.State, error) {
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.Path)
	}

	a.playlist.Replace(paths)

	// Titles and durations come from the file, so the queue reads properly
	// before anything has been opened.
	a.playlist.Restore(items)
	a.emitPlaylist()

	a.log.Info("app: loaded %s with %d items", name, len(items))

	if item, _, ok := a.playlist.Current(); ok {
		if _, err := a.Open(item.Path); err != nil {
			// A first entry that will not open is not a failed load; the rest of
			// the queue is still there to play.
			a.log.Warn("app: the first entry of %s could not be opened: %v", name, err)
			return a.Playlist(), fmt.Errorf("%v", err)
		}
		_ = a.Play()
	}

	return a.Playlist(), nil
}
