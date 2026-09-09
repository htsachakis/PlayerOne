package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"playerone/internal/media"
	"playerone/internal/player"
	"playerone/internal/transcript"
)

// transcriptTimeout bounds subtitle extraction.
//
// Pulling a subtitle stream out of a three-hour file takes a few seconds; a
// minute means the file is damaged or the stream is not what it claims.
const transcriptTimeout = 60 * time.Second

// TranscriptResult is what the Transcript tab renders.
//
// It always carries a Status the interface can show. An empty entry list is a
// legitimate outcome - no subtitles, or an image-based track - and Status is
// what turns that into an explanation instead of a blank panel.
type TranscriptResult struct {
	Entries []transcript.Entry `json:"entries"`

	// TrackID is the subtitle track these entries came from.
	TrackID int `json:"trackId"`
	// TrackLabel names that track for the panel header.
	TrackLabel string `json:"trackLabel"`
	// Source is "embedded" or "external".
	Source string `json:"source"`

	// Available is false when no transcript could be produced.
	Available bool `json:"available"`
	// Status explains the outcome in the user's terms, whether it succeeded or
	// not.
	Status string `json:"status"`
}

// Transcript returns the transcript for the selected subtitle track.
//
// Results are cached per track, so switching back and forth between subtitle
// tracks does not re-run ffmpeg. The cache is dropped whenever the file changes
// or a subtitle is attached.
func (a *App) Transcript() (TranscriptResult, error) {
	engine, err := a.requireEngine()
	if err != nil {
		return TranscriptResult{}, err
	}

	state := engine.State()
	if !state.FileLoaded {
		return TranscriptResult{
			Entries: []transcript.Entry{},
			Status:  "Open a video to see its transcript.",
		}, nil
	}

	trackID := state.SubtitleID
	if trackID == player.NoTrack {
		return TranscriptResult{
			Entries: []transcript.Entry{},
			Status:  "Subtitles are turned off. Choose a subtitle track to see the transcript.",
		}, nil
	}

	if cached := a.cachedTranscript(trackID); cached != nil {
		return *cached, nil
	}

	track, ok := findTrack(engine.Tracks(), player.TrackSubtitle, trackID)
	if !ok {
		return TranscriptResult{
			Entries: []transcript.Entry{},
			Status:  "The selected subtitle track is no longer available.",
		}, nil
	}

	result := a.buildTranscript(state.Path, track)
	a.storeTranscript(trackID, result)
	return result, nil
}

// buildTranscript resolves one subtitle track to transcript entries.
func (a *App) buildTranscript(mediaPath string, track player.Track) TranscriptResult {
	result := TranscriptResult{
		Entries:    []transcript.Entry{},
		TrackID:    track.ID,
		TrackLabel: track.Label,
		Source:     "embedded",
	}
	if track.External {
		result.Source = "external"
	}

	// Image-based subtitles are pictures. Turning them into text would need OCR,
	// which is deliberately out of scope, so this is stated plainly rather than
	// failing with a technical error.
	if track.ImageBased {
		result.Status = fmt.Sprintf(
			"%s is an image-based subtitle track, so it has no text to show. "+
				"Choose a text subtitle track, or load an external .srt file.",
			track.Label)
		return result
	}

	// Cancel any extraction still running for a previously selected track:
	// switching tracks quickly must not leave ffmpeg processes piling up.
	ctx, cancel := context.WithTimeout(a.ctx, transcriptTimeout)
	a.transcriptMu.Lock()
	if a.transcriptCancel != nil {
		a.transcriptCancel()
	}
	a.transcriptCancel = cancel
	a.transcriptMu.Unlock()

	defer func() {
		a.transcriptMu.Lock()
		if a.transcriptCancel != nil {
			a.transcriptCancel()
			a.transcriptCancel = nil
		}
		a.transcriptMu.Unlock()
	}()

	var (
		text string
		err  error
	)

	if track.External && track.ExternalFilename != "" {
		text, err = media.LoadExternal(ctx, a.ffmpegPath, track.ExternalFilename)
	} else {
		text, err = media.ExtractEmbedded(ctx, a.ffmpegPath, mediaPath, track.FFIndex)
	}

	if err != nil {
		a.log.Warn("app: building the transcript for track %d: %v", track.ID, err)

		switch {
		case errors.Is(err, media.ErrImageBased):
			result.Status = fmt.Sprintf(
				"%s appears to be an image-based subtitle track, so it has no text to show.",
				track.Label)
		case a.ffmpegPath == "" && !track.External:
			result.Status = "ffmpeg is needed to read subtitles that are embedded in the video, and it was not found. " +
				"Put ffmpeg.exe in the bin folder next to PlayerOne.exe, or load an external .srt file instead."
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			result.Status = "Reading the subtitle track took too long and was stopped. The track may be damaged."
		default:
			result.Status = fmt.Sprintf("The transcript for %s could not be read.", track.Label)
		}
		return result
	}

	entries, err := transcript.Parse(text)
	if err != nil {
		a.log.Warn("app: parsing the transcript for track %d: %v", track.ID, err)
		result.Status = fmt.Sprintf("%s contains no readable subtitle text.", track.Label)
		return result
	}

	result.Entries = entries
	result.Available = true
	result.Status = fmt.Sprintf("%d lines from %s", len(entries), displayTrackSource(track))

	a.log.Info("app: transcript ready: %d lines from track %d (%s)", len(entries), track.ID, result.Source)
	return result
}

func displayTrackSource(track player.Track) string {
	if track.External && track.ExternalFilename != "" {
		return filepath.Base(track.ExternalFilename)
	}
	return track.Label
}

func findTrack(tracks []player.Track, kind string, id int) (player.Track, bool) {
	for _, t := range tracks {
		if t.Type == kind && t.ID == id {
			return t, true
		}
	}
	return player.Track{}, false
}

func (a *App) cachedTranscript(trackID int) *TranscriptResult {
	a.transcriptMu.Lock()
	defer a.transcriptMu.Unlock()
	return a.transcriptCache[trackID]
}

func (a *App) storeTranscript(trackID int, result TranscriptResult) {
	a.transcriptMu.Lock()
	defer a.transcriptMu.Unlock()
	a.transcriptCache[trackID] = &result
}

// invalidateTranscript drops the cache, which is required whenever the set of
// subtitle tracks could have changed.
func (a *App) invalidateTranscript() {
	a.transcriptMu.Lock()
	a.transcriptCache = make(map[int]*TranscriptResult)
	a.transcriptMu.Unlock()
}

// cancelTranscript stops any extraction in flight, used at shutdown.
func (a *App) cancelTranscript() {
	a.transcriptMu.Lock()
	if a.transcriptCancel != nil {
		a.transcriptCancel()
		a.transcriptCancel = nil
	}
	a.transcriptMu.Unlock()
}
