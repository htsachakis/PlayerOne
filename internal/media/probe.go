// Package media enriches what mpv knows about a file and extracts subtitle text
// for the transcript.
//
// mpv is authoritative for playback; ffprobe fills in the details it does not
// expose (per-stream bitrate, channel layout, creation date), and ffmpeg turns a
// subtitle track into plain timed text. Both are optional: without them playback
// is unaffected and only the Info tab and embedded-subtitle transcripts lose
// detail.
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"playerone/internal/player"
)

// ProbeResult is the part of ffprobe's output PlayerOne uses.
type ProbeResult struct {
	Format  probeFormat   `json:"format"`
	Streams []probeStream `json:"streams"`
}

type probeFormat struct {
	Filename       string            `json:"filename"`
	NBStreams      int               `json:"nb_streams"`
	FormatName     string            `json:"format_name"`
	FormatLongName string            `json:"format_long_name"`
	Duration       string            `json:"duration"`
	Size           string            `json:"size"`
	BitRate        string            `json:"bit_rate"`
	Tags           map[string]string `json:"tags"`
}

type probeStream struct {
	Index         int               `json:"index"`
	CodecName     string            `json:"codec_name"`
	CodecLongName string            `json:"codec_long_name"`
	Profile       string            `json:"profile"`
	CodecType     string            `json:"codec_type"`
	Width         int               `json:"width"`
	Height        int               `json:"height"`
	RFrameRate    string            `json:"r_frame_rate"`
	AvgFrameRate  string            `json:"avg_frame_rate"`
	PixFmt        string            `json:"pix_fmt"`
	BitRate       string            `json:"bit_rate"`
	Channels      int               `json:"channels"`
	ChannelLayout string            `json:"channel_layout"`
	SampleRate    string            `json:"sample_rate"`
	Tags          map[string]string `json:"tags"`
}

// Probe runs ffprobe against a file.
//
// ffprobe is invoked with an argv array, so a path containing spaces or
// non-ASCII characters needs no quoting and no shell is involved.
func Probe(ctx context.Context, ffprobePath, mediaPath string) (*ProbeResult, error) {
	if ffprobePath == "" {
		return nil, fmt.Errorf("media: ffprobe is not available")
	}

	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		mediaPath,
	)
	configureProcAttr(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("media: ffprobe failed on %s: %s", filepath.Base(mediaPath), detail)
	}

	var result ProbeResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("media: reading ffprobe output: %w", err)
	}
	return &result, nil
}

// BuildInfo assembles the Info tab's data from what mpv reports plus, where
// available, ffprobe's extra detail.
//
// mpv's view is preferred wherever the two overlap, because that is what is
// actually being decoded; ffprobe only adds fields mpv does not report.
func BuildInfo(path string, state player.PlaybackState, tracks []player.Track, chapters []player.Chapter, probe *ProbeResult, probeErr error) player.MediaInfo {
	// Every list is initialised, never left nil. Go marshals a nil slice as JSON
	// null, and the interface iterates these directly: one null array is enough
	// to throw inside a render and stop every component that renders after it.
	info := player.MediaInfo{
		Path:      path,
		Filename:  filepath.Base(path),
		Directory: filepath.Dir(path),
		Duration:  state.Duration,
		Title:     state.Title,
		Chapters:  nonNilChapters(chapters),
		Tracks:    nonNilTracks(tracks),
		Audio:     []player.AudioInfo{},
		Subtitles: []player.SubtitleInfo{},
	}

	if stat, err := os.Stat(path); err == nil {
		info.FileSize = stat.Size()
	}

	for _, t := range tracks {
		switch t.Type {
		case player.TrackVideo:
			info.VideoTrackCount++
			if !info.HasVideo {
				info.HasVideo = true
				info.Video = player.VideoInfo{
					Codec:  player.CodecName(t.Codec),
					Width:  t.Width,
					Height: t.Height,
					FPS:    t.FPS,
				}
			}

		case player.TrackAudio:
			info.AudioTrackCount++
			info.Audio = append(info.Audio, player.AudioInfo{
				ID:            t.ID,
				Codec:         player.CodecName(t.Codec),
				Language:      player.LanguageName(t.Language),
				Title:         t.Title,
				Channels:      t.Channels,
				ChannelLayout: player.ChannelLayoutName(t.Channels),
				SampleRate:    t.SampleRate,
				Label:         t.Label,
			})

		case player.TrackSubtitle:
			info.SubtitleTrackCount++
			info.Subtitles = append(info.Subtitles, player.SubtitleInfo{
				ID:         t.ID,
				Codec:      player.CodecName(t.Codec),
				Language:   player.LanguageName(t.Language),
				Title:      t.Title,
				External:   t.External,
				ImageBased: t.ImageBased,
				Label:      t.Label,
			})
		}
	}

	if probeErr != nil {
		info.ProbeError = probeErr.Error()
	}
	if probe != nil {
		applyProbe(&info, probe)
	}

	return info
}

// applyProbe folds ffprobe's detail into an already-built MediaInfo.
func applyProbe(info *player.MediaInfo, probe *ProbeResult) {
	info.Container = probe.Format.FormatName
	info.ContainerLong = probe.Format.FormatLongName

	if info.Duration <= 0 {
		if d, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
			info.Duration = d
		}
	}
	if info.FileSize == 0 {
		if size, err := strconv.ParseInt(probe.Format.Size, 10, 64); err == nil {
			info.FileSize = size
		}
	}
	if bitrate, err := strconv.ParseInt(probe.Format.BitRate, 10, 64); err == nil {
		info.OverallBitrate = bitrate
	}

	if title := lookupTag(probe.Format.Tags, "title"); title != "" {
		info.Title = title
	}
	info.CreationDate = firstTag(probe.Format.Tags, "creation_time", "date", "DATE")

	// Index ffprobe's streams so each mpv track can be matched by its ffmpeg
	// stream index - the same identity used for subtitle extraction.
	byIndex := make(map[int]probeStream, len(probe.Streams))
	for _, s := range probe.Streams {
		byIndex[s.Index] = s
	}

	audioPos := 0
	for _, t := range info.Tracks {
		s, ok := byIndex[t.FFIndex]
		if !ok {
			if t.Type == player.TrackAudio {
				audioPos++
			}
			continue
		}

		switch t.Type {
		case player.TrackVideo:
			if t.Selected || info.Video.Codec == "" {
				info.Video.CodecLong = s.CodecLongName
				info.Video.Profile = s.Profile
				info.Video.PixelFormat = s.PixFmt
				if bitrate, err := strconv.ParseInt(s.BitRate, 10, 64); err == nil {
					info.Video.Bitrate = bitrate
				}
				if info.Video.FPS <= 0 {
					info.Video.FPS = parseFrameRate(s.RFrameRate)
				}
				if info.Video.Width == 0 {
					info.Video.Width, info.Video.Height = s.Width, s.Height
				}
			}

		case player.TrackAudio:
			if audioPos < len(info.Audio) {
				a := &info.Audio[audioPos]
				a.CodecLong = s.CodecLongName
				if s.ChannelLayout != "" {
					a.ChannelLayout = s.ChannelLayout
				}
				if a.SampleRate == 0 {
					if sr, err := strconv.Atoi(s.SampleRate); err == nil {
						a.SampleRate = sr
					}
				}
				if bitrate, err := strconv.ParseInt(s.BitRate, 10, 64); err == nil {
					a.Bitrate = bitrate
				}
			}
			audioPos++
		}
	}

	// A file with no video track at all (an audio lecture) still has a valid
	// overall bitrate, but reporting a video bitrate would be misleading.
	if !info.HasVideo {
		info.Video = player.VideoInfo{}
	}
}

// parseFrameRate converts ffprobe's "30000/1001" rational to frames per second.
func parseFrameRate(value string) float64 {
	num, den, ok := strings.Cut(value, "/")
	if !ok {
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0
		}
		return f
	}

	n, err1 := strconv.ParseFloat(num, 64)
	d, err2 := strconv.ParseFloat(den, 64)
	if err1 != nil || err2 != nil || d == 0 {
		return 0
	}
	return n / d
}

// lookupTag reads a tag case-insensitively.
//
// Containers are inconsistent about tag case - Matroska written by one tool has
// "DATE", another has "date" - so an exact lookup silently loses metadata.
func lookupTag(tags map[string]string, key string) string {
	if tags == nil {
		return ""
	}
	if v, ok := tags[key]; ok {
		return strings.TrimSpace(v)
	}
	for k, v := range tags {
		if strings.EqualFold(k, key) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// firstTag returns the first of several tag names that has a value.
func firstTag(tags map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := lookupTag(tags, k); v != "" {
			return v
		}
	}
	return ""
}

// nonNilChapters and nonNilTracks guarantee an empty JSON array rather than
// null for a file that has none.
func nonNilChapters(in []player.Chapter) []player.Chapter {
	if in == nil {
		return []player.Chapter{}
	}
	return in
}

func nonNilTracks(in []player.Track) []player.Track {
	if in == nil {
		return []player.Track{}
	}
	return in
}
