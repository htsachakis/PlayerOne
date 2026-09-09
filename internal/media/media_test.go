package media

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"playerone/internal/player"
	"playerone/internal/tools"
	"playerone/internal/transcript"
)

func TestIsSubtitleFile(t *testing.T) {
	subtitles := []string{
		`C:\Videos\tutorial.srt`,
		`C:\Videos\tutorial.SRT`,
		"tutorial.vtt",
		"tutorial.ass",
		"tutorial.ssa",
		`C:\path with spaces\name.srt`,
	}
	for _, p := range subtitles {
		if !IsSubtitleFile(p) {
			t.Errorf("IsSubtitleFile(%q) = false, want true", p)
		}
	}

	others := []string{"movie.mkv", "movie.mp4", "notes.txt", "archive.zip", "noextension", ""}
	for _, p := range others {
		if IsSubtitleFile(p) {
			t.Errorf("IsSubtitleFile(%q) = true, want false", p)
		}
	}
}

func TestSubtitleExtensionsCoversTheDocumentedFormats(t *testing.T) {
	got := map[string]bool{}
	for _, ext := range SubtitleExtensions() {
		got[ext] = true
	}

	for _, want := range []string{".srt", ".ass", ".ssa", ".vtt"} {
		if !got[want] {
			t.Errorf("SubtitleExtensions() is missing %q", want)
		}
	}
}

func TestParseFrameRate(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"30000/1001", 29.97002997002997},
		{"25/1", 25},
		{"24", 24},
		{"0/0", 0},
		{"", 0},
		{"nonsense", 0},
	}

	for _, tc := range cases {
		got := parseFrameRate(tc.in)
		if diff := got - tc.want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("parseFrameRate(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// Matroska tag case varies by muxer, so metadata must be found regardless.
func TestLookupTagIsCaseInsensitive(t *testing.T) {
	tags := map[string]string{"TITLE": "The Real Title", "DATE": "20260513"}

	if got := lookupTag(tags, "title"); got != "The Real Title" {
		t.Errorf("lookupTag(title) = %q", got)
	}
	if got := lookupTag(tags, "date"); got != "20260513" {
		t.Errorf("lookupTag(date) = %q", got)
	}
	if got := lookupTag(nil, "title"); got != "" {
		t.Errorf("lookupTag on nil tags = %q, want empty", got)
	}
}

func TestFirstTagPrefersEarlierKeys(t *testing.T) {
	tags := map[string]string{"DATE": "20260513"}
	if got := firstTag(tags, "creation_time", "date"); got != "20260513" {
		t.Errorf("firstTag = %q, want the date tag", got)
	}
	if got := firstTag(tags, "nothing", "missing"); got != "" {
		t.Errorf("firstTag with no matches = %q, want empty", got)
	}
}

func TestDecodeSubtitleBytes(t *testing.T) {
	t.Run("plain utf-8", func(t *testing.T) {
		if got := decodeSubtitleBytes([]byte("Καλημέρα")); got != "Καλημέρα" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("utf-8 with a byte order mark", func(t *testing.T) {
		raw := append([]byte{0xEF, 0xBB, 0xBF}, []byte("hello")...)
		if got := decodeSubtitleBytes(raw); got != "hello" {
			t.Errorf("got %q, want the BOM stripped", got)
		}
	})

	t.Run("utf-16 little endian", func(t *testing.T) {
		raw := []byte{0xFF, 0xFE, 'h', 0, 'i', 0}
		if got := decodeSubtitleBytes(raw); got != "hi" {
			t.Errorf("got %q, want %q", got, "hi")
		}
	})

	t.Run("utf-16 big endian", func(t *testing.T) {
		raw := []byte{0xFE, 0xFF, 0, 'h', 0, 'i'}
		if got := decodeSubtitleBytes(raw); got != "hi" {
			t.Errorf("got %q, want %q", got, "hi")
		}
	})

	// A legacy single-byte encoding must not produce replacement characters or
	// an error; ASCII has to survive intact.
	t.Run("legacy single-byte encoding", func(t *testing.T) {
		raw := []byte{'C', 'a', 'f', 0xE9} // Latin-1 "Café"
		got := decodeSubtitleBytes(raw)
		if !strings.HasPrefix(got, "Caf") {
			t.Errorf("got %q, want the ASCII part preserved", got)
		}
		if strings.ContainsRune(got, '\uFFFD') {
			t.Errorf("got %q, want no replacement characters", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		if got := decodeSubtitleBytes(nil); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func TestApplyProbeFillsInWhatMPVDoesNotReport(t *testing.T) {
	probe := &ProbeResult{
		Format: probeFormat{
			FormatName:     "matroska,webm",
			FormatLongName: "Matroska / WebM",
			Duration:       "8309.237",
			Size:           "1553036924",
			BitRate:        "1495239",
			Tags:           map[string]string{"title": "Container Title", "DATE": "20260513"},
		},
		Streams: []probeStream{
			{Index: 0, CodecType: "video", CodecName: "h264", CodecLongName: "H.264 / AVC",
				Profile: "High", Width: 1920, Height: 1080, RFrameRate: "30000/1001",
				PixFmt: "yuv420p", BitRate: "1300000"},
			{Index: 1, CodecType: "audio", CodecName: "aac", CodecLongName: "AAC (Advanced Audio Coding)",
				Channels: 2, ChannelLayout: "stereo", SampleRate: "44100", BitRate: "128000"},
			{Index: 2, CodecType: "subtitle", CodecName: "subrip"},
		},
	}

	tracks := []player.Track{
		{ID: 1, Type: player.TrackVideo, Codec: "h264", FFIndex: 0, Width: 1920, Height: 1080, FPS: 29.97, Selected: true},
		{ID: 1, Type: player.TrackAudio, Codec: "aac", FFIndex: 1, Language: "eng", Channels: 2, SampleRate: 44100, Selected: true},
		{ID: 1, Type: player.TrackSubtitle, Codec: "subrip", FFIndex: 2, Language: "eng", Title: "English"},
	}

	info := BuildInfo(`C:\Videos\test.mkv`, player.PlaybackState{Duration: 8309.237, Title: "State Title"}, tracks, nil, probe, nil)

	if info.Container != "matroska,webm" {
		t.Errorf("Container = %q", info.Container)
	}
	if info.Title != "Container Title" {
		t.Errorf("Title = %q, want the container's title to win", info.Title)
	}
	if info.CreationDate != "20260513" {
		t.Errorf("CreationDate = %q", info.CreationDate)
	}
	if info.OverallBitrate != 1495239 {
		t.Errorf("OverallBitrate = %d", info.OverallBitrate)
	}

	if !info.HasVideo {
		t.Fatal("HasVideo = false, want true")
	}
	if info.Video.Codec != "H.264" {
		t.Errorf("Video.Codec = %q, want the friendly name", info.Video.Codec)
	}
	if info.Video.Profile != "High" || info.Video.PixelFormat != "yuv420p" {
		t.Errorf("Video profile/pixfmt = %q/%q", info.Video.Profile, info.Video.PixelFormat)
	}
	if info.Video.Bitrate != 1300000 {
		t.Errorf("Video.Bitrate = %d", info.Video.Bitrate)
	}

	if len(info.Audio) != 1 {
		t.Fatalf("got %d audio entries, want 1", len(info.Audio))
	}
	if info.Audio[0].ChannelLayout != "stereo" {
		t.Errorf("Audio ChannelLayout = %q, want ffprobe's value", info.Audio[0].ChannelLayout)
	}
	if info.Audio[0].Bitrate != 128000 {
		t.Errorf("Audio.Bitrate = %d", info.Audio[0].Bitrate)
	}
	if info.Audio[0].SampleRate != 44100 {
		t.Errorf("Audio.SampleRate = %d", info.Audio[0].SampleRate)
	}

	if info.VideoTrackCount != 1 || info.AudioTrackCount != 1 || info.SubtitleTrackCount != 1 {
		t.Errorf("track counts = %d/%d/%d, want 1/1/1",
			info.VideoTrackCount, info.AudioTrackCount, info.SubtitleTrackCount)
	}
}

// Without ffprobe the Info tab must still be built from mpv's own view.
func TestBuildInfoWorksWithoutProbe(t *testing.T) {
	tracks := []player.Track{
		{ID: 1, Type: player.TrackVideo, Codec: "h264", Width: 1920, Height: 1080, FPS: 29.97, FFIndex: 0},
		{ID: 1, Type: player.TrackAudio, Codec: "aac", Language: "eng", Channels: 2, FFIndex: 1, Label: "English — AAC Stereo"},
	}
	chapters := []player.Chapter{{Index: 0, Title: "Intro", Start: 0}}

	info := BuildInfo(`C:\Videos\test.mkv`, player.PlaybackState{Duration: 100, Title: "From mpv"}, tracks, chapters, nil, os.ErrNotExist)

	if info.Filename != "test.mkv" {
		t.Errorf("Filename = %q", info.Filename)
	}
	if info.Duration != 100 {
		t.Errorf("Duration = %v", info.Duration)
	}
	if info.Title != "From mpv" {
		t.Errorf("Title = %q, want mpv's title when there is no probe", info.Title)
	}
	if !info.HasVideo || info.Video.Width != 1920 {
		t.Errorf("Video = %+v", info.Video)
	}
	if len(info.Chapters) != 1 {
		t.Errorf("got %d chapters, want 1", len(info.Chapters))
	}
	if info.ProbeError == "" {
		t.Error("ProbeError is empty; the reason for the missing detail should be recorded")
	}
}

func TestBuildInfoOnAnAudioOnlyFile(t *testing.T) {
	tracks := []player.Track{
		{ID: 1, Type: player.TrackAudio, Codec: "mp3", Channels: 2, FFIndex: 0},
	}
	info := BuildInfo(`C:\Audio\lecture.mp3`, player.PlaybackState{Duration: 600}, tracks, nil, nil, nil)

	if info.HasVideo {
		t.Error("HasVideo = true for an audio-only file")
	}
	if info.Video.Codec != "" {
		t.Errorf("Video = %+v, want it left empty", info.Video)
	}
	if info.AudioTrackCount != 1 {
		t.Errorf("AudioTrackCount = %d, want 1", info.AudioTrackCount)
	}
}

func TestProbeResultDecodesRealFFprobeOutput(t *testing.T) {
	// Trimmed from ffprobe's actual output for the test fixture.
	const payload = `{
	  "streams": [
	    {"index":0,"codec_name":"h264","codec_long_name":"H.264","profile":"High","codec_type":"video",
	     "width":1920,"height":1080,"r_frame_rate":"30000/1001","pix_fmt":"yuv420p"},
	    {"index":1,"codec_name":"aac","codec_type":"audio","channels":2,"sample_rate":"44100",
	     "channel_layout":"stereo","tags":{"language":"eng"}},
	    {"index":2,"codec_name":"subrip","codec_type":"subtitle","tags":{"language":"eng","title":"English"}}
	  ],
	  "format": {"filename":"x.mkv","nb_streams":3,"format_name":"matroska,webm",
	   "duration":"8309.237000","size":"1553036924","bit_rate":"1495239",
	   "tags":{"title":"Betaflight PID Tuning Masterclass","GENRE":"Education","DATE":"20260513"}}
	}`

	var result ProbeResult
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		t.Fatalf("decoding ffprobe output: %v", err)
	}

	if len(result.Streams) != 3 {
		t.Fatalf("got %d streams, want 3", len(result.Streams))
	}
	if result.Streams[2].CodecName != "subrip" {
		t.Errorf("stream 2 codec = %q", result.Streams[2].CodecName)
	}
	if result.Format.Duration != "8309.237000" {
		t.Errorf("duration = %q", result.Format.Duration)
	}
	if lookupTag(result.Format.Tags, "date") != "20260513" {
		t.Errorf("date tag not found in %v", result.Format.Tags)
	}
}

func TestExtractEmbeddedRejectsBadInput(t *testing.T) {
	ctx := context.Background()

	if _, err := ExtractEmbedded(ctx, "", "movie.mkv", 2); err == nil {
		t.Error("expected an error when ffmpeg is unavailable")
	}
	if _, err := ExtractEmbedded(ctx, "ffmpeg", "movie.mkv", -1); err == nil {
		t.Error("expected an error for a track with no stream index")
	}
}

func TestLoadExternalRejectsUnsupportedFormats(t *testing.T) {
	if _, err := LoadExternal(context.Background(), "ffmpeg", "notes.txt"); err == nil {
		t.Error("expected an error for a file that is not a subtitle format")
	}
}

func TestLoadExternalReadsAnSRTFileDirectly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.srt")
	const content = "1\n00:00:01,000 --> 00:00:02,000\nHello.\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	// No ffmpeg needed: SubRip is read straight from disk.
	got, err := LoadExternal(context.Background(), "", path)
	if err != nil {
		t.Fatalf("LoadExternal: %v", err)
	}
	if got != content {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestLoadExternalRejectsAnEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.srt")
	if err := os.WriteFile(path, []byte("   \n\n"), 0o644); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	if _, err := LoadExternal(context.Background(), "", path); err == nil {
		t.Error("expected an error for a subtitle file with no text")
	}
}

// --- Integration tests against the real fixture and the real tools ---
//
// These need bin/ffprobe.exe, bin/ffmpeg.exe and a video in testdata/media.
// They skip cleanly when any of those is missing, so `go test ./...` stays green
// on a checkout without them.

func fixtureVideo(t *testing.T) string {
	t.Helper()

	patterns := []string{"*.mkv", "*.mp4", "*.webm"}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join("..", "..", "testdata", "media", pattern))
		if err == nil && len(matches) > 0 {
			return matches[0]
		}
	}

	t.Skip("no video in testdata/media; see the README for where to put test media")
	return ""
}

func resolveTool(t *testing.T, name string) string {
	t.Helper()

	r := tools.NewResolverWithDirs([]string{
		filepath.Join("..", "..", "bin"),
	})
	path, err := r.Look(name)
	if err != nil {
		t.Skipf("%s is not available: %v", name, err)
	}
	return path
}

func TestIntegrationProbeRealFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping an integration test in short mode")
	}

	video := fixtureVideo(t)
	ffprobe := resolveTool(t, tools.FFprobe)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := Probe(ctx, ffprobe, video)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}

	if result.Format.FormatName == "" {
		t.Error("Probe returned no container format")
	}
	if len(result.Streams) == 0 {
		t.Error("Probe returned no streams")
	}

	t.Logf("probed %s: %s, %d streams, duration %s",
		filepath.Base(video), result.Format.FormatName, len(result.Streams), result.Format.Duration)
}

// The full transcript pipeline: find the subtitle stream, extract it with
// ffmpeg, and parse it into entries the panel can show.
func TestIntegrationExtractAndParseEmbeddedSubtitles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping an integration test in short mode")
	}

	video := fixtureVideo(t)
	ffprobe := resolveTool(t, tools.FFprobe)
	ffmpeg := resolveTool(t, tools.FFmpeg)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	probe, err := Probe(ctx, ffprobe, video)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}

	subtitleIndex := -1
	for _, s := range probe.Streams {
		if s.CodecType == "subtitle" && !player.IsImageSubtitle(s.CodecName) {
			subtitleIndex = s.Index
			break
		}
	}
	if subtitleIndex < 0 {
		t.Skip("the fixture has no text subtitle track")
	}

	text, err := ExtractEmbedded(ctx, ffmpeg, video, subtitleIndex)
	if err != nil {
		t.Fatalf("ExtractEmbedded(stream %d): %v", subtitleIndex, err)
	}

	entries, err := transcript.Parse(text)
	if err != nil {
		t.Fatalf("parsing the extracted subtitles: %v", err)
	}
	if len(entries) < 10 {
		t.Fatalf("got %d transcript entries, expected many more", len(entries))
	}

	for i, e := range entries {
		if e.Text == "" {
			t.Fatalf("entry %d has no text", i)
		}
		if e.End < e.Start {
			t.Fatalf("entry %d ends before it starts: %+v", i, e)
		}
	}

	t.Logf("extracted and parsed %d transcript entries from stream %d; first line at %.2fs: %q",
		len(entries), subtitleIndex, entries[0].Start, entries[0].Text)
}
