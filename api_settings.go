package main

import (
	"fmt"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"playerone/internal/history"
	"playerone/internal/logging"
	"playerone/internal/settings"
	"playerone/internal/winvideo"
)

// settingsMutation names settings.Settings at the call sites that mutate it, so
// the persist helper below reads clearly.
type settingsMutation = settings.Settings

// persist applies a change to the settings and writes them out.
//
// A failure to save is logged rather than returned: the change has already taken
// effect in the player, and refusing the user's action because a file could not
// be written would be worse than silently losing the preference.
func (a *App) persist(mutate func(*settingsMutation)) {
	if a.settings == nil {
		return
	}
	if _, err := a.settings.Update(mutate); err != nil {
		a.log.Warn("app: could not save settings: %v", err)
	}
}

// Settings returns the stored preferences.
func (a *App) Settings() settings.Settings {
	if a.settings == nil {
		return settings.Defaults()
	}
	return a.settings.Get()
}

// SaveSettings replaces the stored preferences and applies the ones that affect
// playback immediately.
func (a *App) SaveSettings(next settings.Settings) (settings.Settings, error) {
	if a.settings == nil {
		return next, fmt.Errorf("settings cannot be saved: no configuration directory is available")
	}

	previous := a.settings.Get()
	if err := a.settings.Set(next); err != nil {
		return a.settings.Get(), fmt.Errorf("settings could not be saved: %w", err)
	}
	applied := a.settings.Get()

	a.log.SetLevel(logging.ParseLevel(applied.LogLevel))

	// Push the values that the player owns, so a change in the settings panel
	// takes effect without waiting for the next file.
	if engine := a.currentEngine(); engine != nil {
		if applied.Volume != previous.Volume {
			_ = engine.SetVolume(a.ctx, applied.Volume)
		}
		if applied.Muted != previous.Muted {
			_ = engine.SetMute(a.ctx, applied.Muted)
		}
		if applied.PlaybackSpeed != previous.PlaybackSpeed {
			_ = engine.SetSpeed(a.ctx, applied.PlaybackSpeed)
		}
		if applied.SubtitleDelay != previous.SubtitleDelay {
			_ = engine.SetSubtitleDelay(a.ctx, applied.SubtitleDelay)
		}
		if applied.AudioDelay != previous.AudioDelay {
			_ = engine.SetAudioDelay(a.ctx, applied.AudioDelay)
		}
		if applied.SubtitleScale != previous.SubtitleScale {
			_ = engine.SetSubtitleScale(a.ctx, applied.SubtitleScale)
		}
	}

	return applied, nil
}

// SetFollowTranscript stores whether the transcript should track playback.
func (a *App) SetFollowTranscript(follow bool) {
	a.persist(func(s *settingsMutation) { s.FollowTranscript = follow })
}

// SetAutoResume stores whether saved positions are applied without asking.
func (a *App) SetAutoResume(auto bool) {
	a.persist(func(s *settingsMutation) { s.AutoResume = auto })
}

// SetSidePanel stores the panel's visibility and width.
func (a *App) SetSidePanel(visible bool, width int) {
	a.log.Debug("app: side panel visible=%v width=%d", visible, width)

	a.persist(func(s *settingsMutation) {
		s.SidePanelVisible = visible
		if width > 0 {
			s.SidePanelWidth = width
		}
	})
}

// SaveWindowState stores the window geometry so the next launch matches this one.
func (a *App) SaveWindowState(width, height, x, y int, maximised bool) {
	a.persist(func(s *settingsMutation) {
		s.Window = settings.WindowState{
			Width:     width,
			Height:    height,
			X:         x,
			Y:         y,
			Maximised: maximised,
			Valid:     true,
		}
	})
}

// RecentFiles returns the recently opened files, newest first, each flagged with
// whether it still exists.
func (a *App) RecentFiles() []history.Entry {
	if a.history == nil {
		return []history.Entry{}
	}
	entries := a.history.Recent(history.MaxEntries)
	if entries == nil {
		return []history.Entry{}
	}
	return entries
}

// ForgetRecent removes one file from the recent list.
func (a *App) ForgetRecent(path string) error {
	if a.history == nil {
		return nil
	}
	if err := a.history.Forget(path); err != nil {
		return fmt.Errorf("the recent file could not be removed: %w", err)
	}
	return nil
}

// ClearRecent empties the recent list.
func (a *App) ClearRecent() error {
	if a.history == nil {
		return nil
	}
	if err := a.history.Clear(); err != nil {
		return fmt.Errorf("the recent list could not be cleared: %w", err)
	}
	return nil
}

// SetVideoBounds positions the video surface.
//
// The frontend measures its video slot in CSS pixels and passes the device pixel
// ratio; the conversion to physical pixels happens here because the native
// window works in physical pixels and only the frontend knows the ratio.
//
// This is the mechanism behind fullscreen and control auto-hide: both are
// changes to this rectangle, never an overlay, because a native child window
// cannot be covered by HTML.
func (a *App) SetVideoBounds(x, y, width, height float64, dpr float64) {
	if dpr <= 0 {
		dpr = 1
	}

	bounds := winvideo.Bounds{
		X:      int(x*dpr + 0.5),
		Y:      int(y*dpr + 0.5),
		Width:  int(width*dpr + 0.5),
		Height: int(height*dpr + 0.5),
	}

	// The rectangle is always recorded, even when there is no window to apply it
	// to yet. The interface measures its layout as soon as the page renders,
	// which is normally before the engine has finished starting; dropping that
	// first measurement would leave the video window at its initial 1x1 size,
	// and since the rectangle then never changes again, nothing would ever
	// correct it. The result is audio with no picture.
	a.mu.Lock()
	a.videoBounds = bounds
	a.hasVideoBounds = true
	host := a.videoHost
	a.mu.Unlock()

	if host == nil {
		return
	}
	host.SetBounds(bounds)
}

// applyPendingVideoLayout puts the window where the interface last asked, and is
// called once the window exists.
func (a *App) applyPendingVideoLayout() {
	a.mu.RLock()
	bounds, has := a.videoBounds, a.hasVideoBounds
	visible := a.videoVisible
	host := a.videoHost
	a.mu.RUnlock()

	if host == nil || !has {
		return
	}

	a.log.Debug("app: applying video bounds %+v (visible=%v)", bounds, visible)
	host.SetBounds(bounds)
	if visible {
		host.Show()
	}
}

// SetVideoVisible shows or hides the video surface.
//
// Hiding it is how the interface displays anything that would otherwise be
// covered: the empty state, an error, or the resume prompt.
func (a *App) SetVideoVisible(visible bool) {
	// Recorded unconditionally for the same reason as the bounds: the request
	// can arrive before the window exists.
	a.mu.Lock()
	a.videoVisible = visible
	host := a.videoHost
	a.mu.Unlock()

	if host == nil {
		return
	}
	if visible {
		host.Show()
	} else {
		host.Hide()
	}
}

// SetFullscreen switches the window in and out of fullscreen.
func (a *App) SetFullscreen(full bool) {
	a.mu.Lock()
	a.fullscreen = full
	a.mu.Unlock()

	if full {
		wailsruntime.WindowFullscreen(a.ctx)
		a.startPointerWatch()
	} else {
		wailsruntime.WindowUnfullscreen(a.ctx)
		a.stopPointerWatch()
	}
}

// pointerPollInterval is how often the cursor is sampled in fullscreen.
//
// Eight times a second is far below anything a person would notice as lag when
// reaching for the controls, and is a rounding error next to decoding video.
const pointerPollInterval = 125 * time.Millisecond

// startPointerWatch begins reporting pointer movement while fullscreen.
//
// The video is a native window, so while the pointer is over it the WebView
// receives no mouse events whatsoever. Once the fullscreen controls auto-hide
// the video covers the whole client area, which leaves the page unable to tell
// that the pointer moved - and therefore no way to bring the controls back,
// stranding the viewer with no means of pausing or seeking. Asking Windows for
// the cursor position directly is the only signal that still works there.
func (a *App) startPointerWatch() {
	a.pointerMu.Lock()
	defer a.pointerMu.Unlock()

	if a.pointerStop != nil {
		return // already watching
	}

	stop := make(chan struct{})
	a.pointerStop = stop

	a.log.Debug("app: watching the cursor so fullscreen controls can be woken")

	a.wg.Add(1)
	go a.watchPointer(stop)
}

func (a *App) stopPointerWatch() {
	a.pointerMu.Lock()
	defer a.pointerMu.Unlock()

	if a.pointerStop == nil {
		return
	}
	close(a.pointerStop)
	a.pointerStop = nil
	a.log.Debug("app: stopped watching the cursor")
}

func (a *App) watchPointer(stop chan struct{}) {
	defer a.wg.Done()

	ticker := time.NewTicker(pointerPollInterval)
	defer ticker.Stop()

	lastX, lastY, ok := winvideo.CursorPos()
	if !ok {
		a.log.Debug("app: the cursor position is unavailable; fullscreen controls will wake on key presses only")
		return
	}

	for {
		select {
		case <-stop:
			return
		case <-a.stopCh:
			return
		case <-ticker.C:
			x, y, ok := winvideo.CursorPos()
			if !ok || (x == lastX && y == lastY) {
				continue
			}
			lastX, lastY = x, y
			a.emit(eventPointerMoved)
		}
	}
}

// IsFullscreen reports the window's fullscreen state.
func (a *App) IsFullscreen() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.fullscreen
}
