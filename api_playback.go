package main

import (
	"playerone/internal/player"
)

// This file holds the playback-control half of the frontend API. Every method
// here returns an error the UI can show directly, and none of them block on
// anything slower than a local IPC round trip.

// PlayPause toggles between playing and paused.
func (a *App) PlayPause() error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}
	return engine.TogglePause(a.ctx)
}

// Play resumes playback.
func (a *App) Play() error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}
	return engine.Play(a.ctx)
}

// Pause pauses playback and records the position, since a pause is a natural
// moment to checkpoint.
func (a *App) Pause() error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}
	if err := engine.Pause(a.ctx); err != nil {
		return err
	}
	a.saveResumePosition()
	return nil
}

// Stop closes the current file and returns to the empty state.
func (a *App) Stop() error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}

	a.saveResumePosition()

	if err := engine.Unload(a.ctx); err != nil {
		return err
	}

	// The video window must go away, or it would cover the empty state with a
	// black rectangle.
	if host := a.host(); host != nil {
		host.Hide()
	}

	a.mu.Lock()
	a.currentPath = ""
	a.mediaInfo = nil
	a.mu.Unlock()

	a.invalidateTranscript()
	a.emit(eventMediaOpen, nil)
	return nil
}

// Seek moves to an absolute position in seconds.
func (a *App) Seek(seconds float64) error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}
	return engine.Seek(a.ctx, seconds)
}

// SeekRelative moves by an offset in seconds, positive or negative.
func (a *App) SeekRelative(delta float64) error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}
	return engine.SeekRelative(a.ctx, delta)
}

// SeekChapter jumps to a chapter by index.
func (a *App) SeekChapter(index int) error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}
	return engine.SeekChapter(a.ctx, index)
}

// SetVolume sets the volume as a percentage and remembers it.
func (a *App) SetVolume(value float64) error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}
	if err := engine.SetVolume(a.ctx, value); err != nil {
		return err
	}

	a.persist(func(s *settingsMutation) { s.Volume = value })
	return nil
}

// SetMute mutes or unmutes.
func (a *App) SetMute(muted bool) error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}
	if err := engine.SetMute(a.ctx, muted); err != nil {
		return err
	}

	a.persist(func(s *settingsMutation) { s.Muted = muted })
	return nil
}

// ToggleMute flips the mute state.
func (a *App) ToggleMute() error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}
	return a.SetMute(!engine.State().Muted)
}

// SetSpeed sets the playback rate. Values from 0.1 up to 16 are accepted; the
// interface offers 0.25 through 8.
func (a *App) SetSpeed(speed float64) error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}
	if err := engine.SetSpeed(a.ctx, speed); err != nil {
		return err
	}

	a.persist(func(s *settingsMutation) { s.PlaybackSpeed = speed })
	return nil
}

// SpeedPresets are the rates offered in the interface.
//
// Everything above 2x is there for skimming a tutorial rather than watching it,
// which is why the list runs so much further than a general-purpose player's.
func (a *App) SpeedPresets() []float64 {
	return []float64{0.25, 0.5, 0.75, 1.0, 1.25, 1.5, 1.75, 2.0, 3.0, 4.0, 8.0}
}

// SetReverse switches between forward and backward playback.
//
// This is mpv's genuine reverse playback, not repeated backward seeking, so the
// picture stays continuous. mpv documents it as CPU-intensive and it will not
// work on every file; a failure is returned so the interface can say so and
// switch back.
func (a *App) SetReverse(reverse bool) error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}
	if err := engine.SetReverse(a.ctx, reverse); err != nil {
		return err
	}

	a.log.Info("app: playback direction is now %s", directionName(reverse))
	return nil
}

// ToggleReverse flips the playback direction.
func (a *App) ToggleReverse() error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}
	return a.SetReverse(!engine.State().Reverse)
}

func directionName(reverse bool) string {
	if reverse {
		return "backward"
	}
	return "forward"
}

// SetSubtitleTrack selects a subtitle track by mpv track ID.
func (a *App) SetSubtitleTrack(id int) error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}
	if err := engine.SetSubtitleTrack(a.ctx, id); err != nil {
		return err
	}

	a.rememberSubtitleLanguage(engine.Tracks(), id)
	a.invalidateTranscript()
	return nil
}

// DisableSubtitles turns subtitles off entirely.
func (a *App) DisableSubtitles() error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}
	if err := engine.DisableSubtitles(a.ctx); err != nil {
		return err
	}

	a.persist(func(s *settingsMutation) { s.LastSubtitleLang = subtitlesOff })
	a.invalidateTranscript()
	return nil
}

// SetAudioTrack selects an audio track by mpv track ID.
func (a *App) SetAudioTrack(id int) error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}
	if err := engine.SetAudioTrack(a.ctx, id); err != nil {
		return err
	}

	for _, t := range engine.Tracks() {
		if t.Type == player.TrackAudio && t.ID == id {
			a.persist(func(s *settingsMutation) { s.LastAudioLang = t.Language })
			break
		}
	}
	return nil
}

func (a *App) rememberSubtitleLanguage(tracks []player.Track, id int) {
	for _, t := range tracks {
		if t.Type == player.TrackSubtitle && t.ID == id {
			a.persist(func(s *settingsMutation) { s.LastSubtitleLang = t.Language })
			return
		}
	}
}

// SetSubtitleDelay shifts subtitles in seconds; positive shows them later.
func (a *App) SetSubtitleDelay(seconds float64) error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}
	if err := engine.SetSubtitleDelay(a.ctx, seconds); err != nil {
		return err
	}

	a.persist(func(s *settingsMutation) { s.SubtitleDelay = seconds })
	return nil
}

// SetAudioDelay shifts audio in seconds; positive delays the sound.
func (a *App) SetAudioDelay(seconds float64) error {
	engine, err := a.requireEngine()
	if err != nil {
		return err
	}
	if err := engine.SetAudioDelay(a.ctx, seconds); err != nil {
		return err
	}

	a.persist(func(s *settingsMutation) { s.AudioDelay = seconds })
	return nil
}

// State returns the current playback snapshot.
//
// The frontend receives state through events; this exists so a freshly loaded
// page can populate itself without waiting for the next tick.
func (a *App) State() player.PlaybackState {
	engine := a.currentEngine()
	if engine == nil {
		return player.PlaybackState{Paused: true, Idle: true, ChapterIndex: -1, Volume: a.settings.Get().Volume, Speed: 1}
	}
	return engine.State()
}

// Tracks returns the current track list.
func (a *App) Tracks() []player.Track {
	engine := a.currentEngine()
	if engine == nil {
		return []player.Track{}
	}
	tracks := engine.Tracks()
	if tracks == nil {
		return []player.Track{}
	}
	return tracks
}

// Chapters returns the current chapter list.
func (a *App) Chapters() []player.Chapter {
	engine := a.currentEngine()
	if engine == nil {
		return []player.Chapter{}
	}
	chapters := engine.Chapters()
	if chapters == nil {
		return []player.Chapter{}
	}
	return chapters
}
