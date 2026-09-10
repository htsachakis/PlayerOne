package player

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"playerone/internal/logging"
	"playerone/internal/mpvipc"
)

// applyProperty folds one mpv property-change into the player's state.
//
// Runs on the IPC reader goroutine, so it only touches guarded state and never
// calls out. A property mpv reports as null means "no value right now" — for
// instance time-pos while idle — and leaves the previous value alone rather than
// resetting it to zero, which would make the seek bar jump on every track
// change.
func (p *MPVPlayer) applyProperty(name string, data json.RawMessage) {
	if isNull(data) {
		p.applyNullProperty(name)
		return
	}

	switch name {
	case "time-pos":
		if v, ok := decodeFloat(data); ok {
			p.mu.Lock()
			p.state.Position = v
			p.state.ChapterIndex = ChapterAt(p.chapters, v)
			p.mu.Unlock()
			p.dirty.Store(true)
		}

	case "duration":
		p.setFloat(&p.state.Duration, data)

	case "pause":
		p.setBool(&p.state.Paused, data)

	case "volume":
		p.setFloat(&p.state.Volume, data)

	case "mute":
		p.setBool(&p.state.Muted, data)

	case "speed":
		p.setFloat(&p.state.Speed, data)

	case "play-dir":
		if v, ok := decodeString(data); ok {
			reverse := v == "backward" || v == "-"
			p.mu.Lock()
			p.state.Reverse = reverse
			p.mu.Unlock()
			p.dirty.Store(true)
		}

	case "chapter":
		if v, ok := decodeInt(data); ok {
			p.mu.Lock()
			p.state.ChapterIndex = v
			p.mu.Unlock()
			p.dirty.Store(true)
		}

	case "chapter-list":
		p.applyChapterList(data)

	case "track-list":
		p.applyTrackList(data)

	case "media-title":
		if v, ok := decodeString(data); ok && v != "" {
			p.mu.Lock()
			p.state.Title = v
			p.mu.Unlock()
			p.dirty.Store(true)
		}

	case "path":
		if v, ok := decodeString(data); ok && v != "" {
			p.mu.Lock()
			p.state.Path = v
			p.mu.Unlock()
			p.dirty.Store(true)
		}

	case "eof-reached":
		// This, not the end-file event, is what marks the end of playback here:
		// --keep-open holds the file open on its last frame and mpv does not
		// emit end-file at all. Only the transition into EOF is reported, so a
		// file that sits paused at its end does not advance repeatedly.
		if reached, ok := decodeBool(data); ok {
			p.mu.Lock()
			was := p.state.EOF
			p.state.EOF = reached
			p.mu.Unlock()
			p.dirty.Store(true)

			if reached && !was {
				p.send(pumpMsg{kind: pumpEndFile, text: "eof"})
			}
		}

	case "idle-active":
		if v, ok := decodeBool(data); ok {
			p.mu.Lock()
			p.state.Idle = v
			if v {
				p.state.FileLoaded = false
			}
			p.mu.Unlock()
			p.dirty.Store(true)
		}

	case "seeking":
		p.setBool(&p.state.Seeking, data)

	case "sid":
		p.setTrackID(&p.state.SubtitleID, data)

	case "aid":
		p.setTrackID(&p.state.AudioID, data)

	case "sub-delay":
		p.setFloat(&p.state.SubtitleDelay, data)

	case "audio-delay":
		p.setFloat(&p.state.AudioDelay, data)

	case "sub-scale":
		p.setFloat(&p.state.SubtitleScale, data)

	case "hwdec-current":
		if v, ok := decodeString(data); ok {
			p.mu.Lock()
			p.state.HWDec = v
			p.mu.Unlock()
			p.dirty.Store(true)
		}
	}
}

// syncedProperties are read back from mpv once a file is open.
//
// Observers alone are not enough: mpv emits a property-change only when a value
// actually changes, so any property that already holds the right value when the
// file loads never produces an event. The most visible casualty is "pause" —
// mpv begins playing a newly loaded file without changing its pause property,
// which would leave the play button showing the wrong icon for the whole
// session.
var syncedProperties = []string{
	"pause",
	"duration",
	"volume",
	"mute",
	"speed",
	"play-dir",
	"sid",
	"aid",
	"sub-delay",
	"audio-delay",
	"sub-scale",
	"hwdec-current",
	"media-title",
	"time-pos",
}

// syncFromMPV reads the current value of every property whose event may not
// have fired, and folds it into the state.
//
// Runs on the pump goroutine. A property that is unavailable is skipped rather
// than treated as an error: several of these legitimately have no value for an
// audio-only file or before the first frame is shown.
func (p *MPVPlayer) syncFromMPV() {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	for _, name := range syncedProperties {
		data, err := p.client.Command(ctx, "get_property", name)
		if err != nil {
			if !mpvipc.IsUnavailable(err) {
				p.log.Debug("player: could not read %q while syncing: %v", name, err)
			}
			continue
		}
		p.applyProperty(name, data)
	}
}

// applyNullProperty handles the properties whose absence is itself meaningful.
func (p *MPVPlayer) applyNullProperty(name string) {
	switch name {
	case "sid":
		p.mu.Lock()
		p.state.SubtitleID = NoTrack
		p.mu.Unlock()
		p.dirty.Store(true)

	case "aid":
		p.mu.Lock()
		p.state.AudioID = NoTrack
		p.mu.Unlock()
		p.dirty.Store(true)

	case "chapter":
		p.mu.Lock()
		p.state.ChapterIndex = -1
		p.mu.Unlock()
		p.dirty.Store(true)
	}
	// Everything else keeps its last value; see the doc comment above.
}

// mpvTrack mirrors one entry of mpv's track-list property.
type mpvTrack struct {
	ID               int     `json:"id"`
	Type             string  `json:"type"`
	Title            string  `json:"title"`
	Lang             string  `json:"lang"`
	Codec            string  `json:"codec"`
	Default          bool    `json:"default"`
	Forced           bool    `json:"forced"`
	Selected         bool    `json:"selected"`
	External         bool    `json:"external"`
	ExternalFilename string  `json:"external-filename"`
	FFIndex          *int    `json:"ff-index"`
	DemuxW           int     `json:"demux-w"`
	DemuxH           int     `json:"demux-h"`
	DemuxFPS         float64 `json:"demux-fps"`
	DemuxChannels    int     `json:"demux-channel-count"`
	DemuxSampleRate  int     `json:"demux-samplerate"`
}

func (p *MPVPlayer) applyTrackList(data json.RawMessage) {
	var raw []mpvTrack
	if err := json.Unmarshal(data, &raw); err != nil {
		p.log.Warn("player: could not read mpv's track list: %v", err)
		return
	}

	tracks := make([]Track, 0, len(raw))
	for _, r := range raw {
		t := Track{
			ID:               r.ID,
			Type:             r.Type,
			Language:         r.Lang,
			Title:            r.Title,
			Codec:            r.Codec,
			Selected:         r.Selected,
			Default:          r.Default,
			External:         r.External,
			ExternalFilename: r.ExternalFilename,
			FFIndex:          -1,
			Channels:         r.DemuxChannels,
			SampleRate:       r.DemuxSampleRate,
			Width:            r.DemuxW,
			Height:           r.DemuxH,
			FPS:              r.DemuxFPS,
		}
		if r.FFIndex != nil {
			t.FFIndex = *r.FFIndex
		}
		if t.Type == TrackSubtitle {
			t.ImageBased = IsImageSubtitle(r.Codec)
		}

		// A forced subtitle track is worth marking, since picking it by accident
		// yields a track that only shows a handful of lines.
		if r.Forced && t.Type == TrackSubtitle && t.Title == "" {
			t.Title = "Forced"
		}

		t.Label = Label(t)
		tracks = append(tracks, t)
	}

	p.mu.Lock()
	p.tracks = tracks
	p.mu.Unlock()

	p.dirty.Store(true)
	p.send(pumpMsg{kind: pumpTracks})

	// summariseTracks walks every track, so it is only built when it will be
	// printed.
	if p.log.Enabled(logging.LevelDebug) {
		p.log.Debug("player: %d tracks: %s", len(tracks), summariseTracks(tracks))
	}
}

// mpvChapter mirrors one entry of mpv's chapter-list property.
type mpvChapter struct {
	Title string  `json:"title"`
	Time  float64 `json:"time"`
}

func (p *MPVPlayer) applyChapterList(data json.RawMessage) {
	var raw []mpvChapter
	if err := json.Unmarshal(data, &raw); err != nil {
		p.log.Warn("player: could not read mpv's chapter list: %v", err)
		return
	}

	chapters := make([]Chapter, 0, len(raw))
	for i, r := range raw {
		title := strings.TrimSpace(r.Title)
		if title == "" {
			// A chapter with no title is still navigable, and an empty row in
			// the list would look broken.
			title = fmt.Sprintf("Chapter %d", i+1)
		}
		chapters = append(chapters, Chapter{Index: i, Title: title, Start: r.Time})
	}

	p.mu.Lock()
	p.chapters = chapters
	p.state.ChapterIndex = ChapterAt(chapters, p.state.Position)
	p.mu.Unlock()

	p.dirty.Store(true)
	p.send(pumpMsg{kind: pumpTracks})

	p.log.Debug("player: %d chapters", len(chapters))
}

// setFloat, setBool and setTrackID are the guarded write helpers. They take a
// pointer into p.state, which is only valid while the lock is held, so the lock
// is taken inside rather than by the caller.
func (p *MPVPlayer) setFloat(field *float64, data json.RawMessage) {
	v, ok := decodeFloat(data)
	if !ok {
		return
	}
	p.mu.Lock()
	*field = v
	p.mu.Unlock()
	p.dirty.Store(true)
}

func (p *MPVPlayer) setBool(field *bool, data json.RawMessage) {
	v, ok := decodeBool(data)
	if !ok {
		return
	}
	p.mu.Lock()
	*field = v
	p.mu.Unlock()
	p.dirty.Store(true)
}

// setTrackID decodes mpv's sid/aid, which is either a track number or the
// boolean false meaning "no track selected".
func (p *MPVPlayer) setTrackID(field *int, data json.RawMessage) {
	value := NoTrack
	if v, ok := decodeInt(data); ok {
		value = v
	} else if b, ok := decodeBool(data); ok && !b {
		value = NoTrack
	} else if s, ok := decodeString(data); ok && s == "no" {
		value = NoTrack
	} else {
		return // unrecognised shape; leave the previous value alone
	}

	p.mu.Lock()
	*field = value
	p.mu.Unlock()
	p.dirty.Store(true)
}

func isNull(data json.RawMessage) bool {
	return len(data) == 0 || string(data) == "null"
}

func decodeFloat(data json.RawMessage) (float64, bool) {
	var v float64
	if err := json.Unmarshal(data, &v); err != nil {
		return 0, false
	}
	return v, true
}

func decodeInt(data json.RawMessage) (int, bool) {
	var v int
	if err := json.Unmarshal(data, &v); err != nil {
		return 0, false
	}
	return v, true
}

func decodeBool(data json.RawMessage) (bool, bool) {
	var v bool
	if err := json.Unmarshal(data, &v); err != nil {
		return false, false
	}
	return v, true
}

func decodeString(data json.RawMessage) (string, bool) {
	var v string
	if err := json.Unmarshal(data, &v); err != nil {
		return "", false
	}
	return v, true
}

// summariseTracks renders the track list for a debug log line.
func summariseTracks(tracks []Track) string {
	var b strings.Builder
	for i, t := range tracks {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s#%d %q", t.Type, t.ID, t.Label)
		if t.Selected {
			b.WriteString(" [selected]")
		}
	}
	return b.String()
}

// endFileReason extracts mpv's reason from an end-file event.
//
// An unreadable event is reported as "unknown" rather than "eof", because
// treating an unknown stop as a natural end would auto-advance the playlist at
// the wrong moment.
func endFileReason(raw json.RawMessage) string {
	var payload struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Reason == "" {
		return "unknown"
	}
	return payload.Reason
}
