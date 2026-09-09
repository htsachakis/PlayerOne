package player

import (
	"encoding/json"
	"testing"
)

func TestLanguageName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"eng", "English"},
		{"en", "English"},
		{"gre", "Greek"},   // ISO 639-2/B, what Matroska usually stores
		{"ell", "Greek"},   // ISO 639-2/T, what some muxers store
		{"el", "Greek"},    // ISO 639-1
		{"spa", "Spanish"},
		{"ger", "German"},
		{"deu", "German"},
		{"fre", "French"},
		{"fra", "French"},
		{"jpn", "Japanese"},
		{"und", "Unknown"},
		{"EN", "English"},        // case-insensitive
		{"pt-BR", "Portuguese (BR)"}, // region subtag preserved
		{"zh-Hans", "Chinese (HANS)"},
		{"en_US", "English (US)"},   // underscore form
		{"", ""},
		{"qqq", "QQQ"}, // unknown codes are shown, not swallowed
	}

	for _, tc := range cases {
		if got := LanguageName(tc.in); got != tc.want {
			t.Errorf("LanguageName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCodecName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"aac", "AAC"},
		{"ac3", "AC3"},
		{"eac3", "E-AC3"},
		{"h264", "H.264"},
		{"hevc", "H.265"},
		{"subrip", "SRT"},
		{"ass", "ASS"},
		{"webvtt", "WebVTT"},
		{"hdmv_pgs_subtitle", "PGS"},
		{"pcm_s24le", "PCM"},
		{"pcm_f64le", "PCM"}, // not listed explicitly, caught by the prefix rule
		{"weirdcodec", "WEIRDCODEC"},
		{"", ""},
	}

	for _, tc := range cases {
		if got := CodecName(tc.in); got != tc.want {
			t.Errorf("CodecName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsImageSubtitle(t *testing.T) {
	for _, codec := range []string{"hdmv_pgs_subtitle", "dvd_subtitle", "dvb_subtitle", "xsub", "VobSub"} {
		if !IsImageSubtitle(codec) {
			t.Errorf("IsImageSubtitle(%q) = false, want true", codec)
		}
	}
	for _, codec := range []string{"subrip", "ass", "webvtt", "mov_text", ""} {
		if IsImageSubtitle(codec) {
			t.Errorf("IsImageSubtitle(%q) = true, want false", codec)
		}
	}
}

func TestChannelLayoutName(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, ""},
		{-1, ""},
		{1, "Mono"},
		{2, "Stereo"},
		{6, "5.1"},
		{8, "7.1"},
		{4, "4 ch"},
	}

	for _, tc := range cases {
		if got := ChannelLayoutName(tc.in); got != tc.want {
			t.Errorf("ChannelLayoutName(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The labels in the specification are the acceptance criteria for this.
func TestLabelForAudioMatchesTheSpecifiedExamples(t *testing.T) {
	cases := []struct {
		name  string
		track Track
		want  string
	}{
		{
			name:  "English AAC stereo",
			track: Track{ID: 1, Type: TrackAudio, Language: "eng", Codec: "aac", Channels: 2},
			want:  "English — AAC Stereo",
		},
		{
			name:  "Greek AAC stereo",
			track: Track{ID: 2, Type: TrackAudio, Language: "gre", Codec: "aac", Channels: 2},
			want:  "Greek — AAC Stereo",
		},
		{
			name:  "English commentary in AC3 5.1",
			track: Track{ID: 3, Type: TrackAudio, Language: "eng", Title: "Commentary", Codec: "ac3", Channels: 6},
			want:  "English Commentary — AC3 5.1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LabelForAudio(tc.track); got != tc.want {
				t.Errorf("LabelForAudio = %q, want %q", got, tc.want)
			}
		})
	}
}

// A title that already names the language must not be doubled up.
func TestLabelDoesNotRepeatTheLanguage(t *testing.T) {
	cases := []struct {
		name  string
		track Track
		want  string
	}{
		{
			name:  "title equals the language",
			track: Track{ID: 1, Type: TrackSubtitle, Language: "eng", Title: "English", Codec: "subrip"},
			want:  "English — SRT",
		},
		{
			name:  "title starts with the language",
			track: Track{ID: 1, Type: TrackSubtitle, Language: "eng", Title: "English (SDH)", Codec: "subrip"},
			want:  "English (SDH) — SRT",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Label(tc.track); got != tc.want {
				t.Errorf("Label = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLabelForSubtitle(t *testing.T) {
	cases := []struct {
		name  string
		track Track
		want  string
	}{
		{
			name:  "plain embedded track",
			track: Track{ID: 1, Type: TrackSubtitle, Language: "spa", Codec: "subrip"},
			want:  "Spanish — SRT",
		},
		{
			name:  "image based tracks say so",
			track: Track{ID: 2, Type: TrackSubtitle, Language: "eng", Codec: "hdmv_pgs_subtitle", ImageBased: true},
			want:  "English — PGS (image)",
		},
		{
			name:  "external file is marked",
			track: Track{ID: 3, Type: TrackSubtitle, Language: "eng", Codec: "subrip", External: true},
			want:  "English — SRT (external)",
		},
		{
			name:  "no language and no title still names the track",
			track: Track{ID: 4, Type: TrackSubtitle, Codec: "ass"},
			want:  "Subtitle track 4 — ASS",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LabelForSubtitle(tc.track); got != tc.want {
				t.Errorf("LabelForSubtitle = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLabelForVideo(t *testing.T) {
	got := LabelForVideo(Track{ID: 1, Type: TrackVideo, Codec: "h264", Width: 1920, Height: 1080})
	if got != "H.264 1920x1080" {
		t.Errorf("LabelForVideo = %q, want %q", got, "H.264 1920x1080")
	}
}

func TestResolutionName(t *testing.T) {
	cases := []struct {
		w, h int
		want string
	}{
		{1920, 1080, "1080p"},
		{3840, 2160, "4K"},
		{2560, 1440, "1440p"},
		{1280, 720, "720p"},
		{640, 480, "480p"},
		{7680, 4320, "8K"},
		{0, 0, ""},
	}

	for _, tc := range cases {
		if got := ResolutionName(tc.w, tc.h); got != tc.want {
			t.Errorf("ResolutionName(%d, %d) = %q, want %q", tc.w, tc.h, got, tc.want)
		}
	}
}

// ChapterAt drives the highlighted row in the chapter list, so its boundary
// behaviour is what the user actually sees.
func TestChapterAt(t *testing.T) {
	// The opening chapters of the real fixture.
	chapters := []Chapter{
		{Index: 0, Title: "Learning PID tuning from the MASTER", Start: 0},
		{Index: 1, Title: "What are PIDs?", Start: 180},
		{Index: 2, Title: "Make sure the drone is mechanically sound", Start: 257},
		{Index: 3, Title: "Check CPU", Start: 342},
		{Index: 4, Title: "RC Link Preset", Start: 420},
	}

	cases := []struct {
		name     string
		position float64
		want     int
	}{
		{"at the very start", 0, 0},
		{"inside the first chapter", 100, 0},
		{"one second before the second chapter", 179, 0},
		{"exactly on a chapter boundary", 180, 1},
		{"inside the second chapter", 200, 1},
		{"inside the last chapter", 500, 4},
		{"far past the last chapter start", 9999, 4},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ChapterAt(chapters, tc.position); got != tc.want {
				t.Errorf("ChapterAt(%v) = %d, want %d", tc.position, got, tc.want)
			}
		})
	}

	if got := ChapterAt(nil, 100); got != -1 {
		t.Errorf("ChapterAt(nil) = %d, want -1", got)
	}
}

// Seeking to a chapter lands microseconds past its start; the highlight must not
// fall back to the previous chapter when it does.
func TestChapterAtAbsorbsSeekOvershoot(t *testing.T) {
	chapters := []Chapter{{Index: 0, Start: 0}, {Index: 1, Start: 180}}

	if got := ChapterAt(chapters, 179.98); got != 1 {
		t.Errorf("ChapterAt(179.98) = %d, want 1 - a seek that lands just short must still count", got)
	}
}

// mpv's track-list is the shape everything downstream depends on, so decoding it
// is worth pinning against a real payload.
func TestTrackListDecodingFromRealMPVPayload(t *testing.T) {
	// Captured from mpv 0.41 for the test fixture.
	const payload = `[
	  {"id":1,"type":"video","src-id":0,"default":true,"forced":false,"selected":true,
	   "external":false,"codec":"h264","ff-index":0,"demux-w":1920,"demux-h":1080,"demux-fps":29.97002997002997},
	  {"id":1,"type":"audio","src-id":1,"lang":"eng","default":true,"forced":false,"selected":true,
	   "external":false,"codec":"aac","ff-index":1,"demux-channel-count":2,"demux-samplerate":44100},
	  {"id":1,"type":"sub","src-id":2,"title":"English","lang":"eng","default":false,"forced":false,
	   "selected":false,"external":false,"codec":"subrip","ff-index":2}
	]`

	var raw []mpvTrack
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		t.Fatalf("decoding the track list: %v", err)
	}
	if len(raw) != 3 {
		t.Fatalf("got %d tracks, want 3", len(raw))
	}

	if raw[0].FFIndex == nil || *raw[0].FFIndex != 0 {
		t.Errorf("video ff-index = %v, want 0", raw[0].FFIndex)
	}
	if raw[1].DemuxChannels != 2 || raw[1].DemuxSampleRate != 44100 {
		t.Errorf("audio channels/rate = %d/%d, want 2/44100", raw[1].DemuxChannels, raw[1].DemuxSampleRate)
	}
	if raw[2].FFIndex == nil || *raw[2].FFIndex != 2 {
		t.Errorf("subtitle ff-index = %v, want 2 - extraction depends on this", raw[2].FFIndex)
	}
	if raw[2].Title != "English" || raw[2].Lang != "eng" {
		t.Errorf("subtitle title/lang = %q/%q", raw[2].Title, raw[2].Lang)
	}
}

func TestChapterListDecoding(t *testing.T) {
	const payload = `[
	  {"title":"Learning PID tuning from the MASTER","time":0.0},
	  {"title":"What are PIDs?","time":180.0},
	  {"title":"","time":257.0}
	]`

	var raw []mpvChapter
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		t.Fatalf("decoding the chapter list: %v", err)
	}
	if len(raw) != 3 {
		t.Fatalf("got %d chapters, want 3", len(raw))
	}
	if raw[1].Title != "What are PIDs?" || raw[1].Time != 180 {
		t.Errorf("chapter 1 = %+v", raw[1])
	}
}

func TestClampSpeed(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{1.0, 1.0},
		{0.5, 0.5},
		{2.0, 2.0},
		{8.0, 8.0}, // the fastest preset must survive
		{0, 1.0},
		{-2, 1.0},
		{1000, maxSpeed},
	}

	for _, tc := range cases {
		if got := clampSpeed(tc.in); got != tc.want {
			t.Errorf("clampSpeed(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}

	zero := 0.0
	if got := clampSpeed(zero / zero); got != 1.0 {
		t.Errorf("clampSpeed(NaN) = %v, want 1.0", got)
	}
}

func TestClampVolume(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{100, 100},
		{0, 0},
		{150, 150},
		{-5, 100},
		{9999, 150},
	}

	for _, tc := range cases {
		if got := clampVolume(tc.in); got != tc.want {
			t.Errorf("clampVolume(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// mpv must never be launched through a shell, and the flags that make the
// embedding and the UI contract work must be present.
func TestMPVArgs(t *testing.T) {
	args := mpvArgs(Config{
		WindowID:      0x1234,
		InitialVolume: 80,
		InitialSpeed:  1.5,
		InitialMuted:  true,
	}, `\\.\pipe\playerone-test`)

	index := map[string]bool{}
	for _, a := range args {
		index[a] = true
	}

	required := []string{
		"--idle=yes",                            // one mpv for the whole session
		"--no-config",                           // ignore the user's mpv.conf
		`--input-ipc-server=\\.\pipe\playerone-test`,
		"--wid=4660",                            // 0x1234, the embedding itself
		"--no-osc",                              // PlayerOne draws the interface
		"--no-input-default-bindings",           // shortcuts must not double-fire
		"--input-vo-keyboard=no",
		"--hr-seek=yes",                         // chapter and transcript accuracy
		"--sub-auto=fuzzy",                      // pick up video.en.srt
		"--keep-open=yes",
		"--audio-pitch-correction=yes",          // intelligible speech at 2x
		"--mute=yes",
		"--volume=80",
		"--speed=1.5",
	}

	for _, want := range required {
		if !index[want] {
			t.Errorf("mpv arguments are missing %q\ngot: %v", want, args)
		}
	}

	// Backward playback needs demuxer history reserved up front.
	var sawBackBytes bool
	for _, a := range args {
		if a == "--demuxer-max-back-bytes=256MiB" {
			sawBackBytes = true
		}
	}
	if !sawBackBytes {
		t.Error("mpv arguments do not reserve demuxer history, so reverse playback will not work")
	}
}

func TestMPVArgsWithoutAWindowOpensItsOwn(t *testing.T) {
	args := mpvArgs(Config{InitialVolume: 100, InitialSpeed: 1}, "pipe")

	for _, a := range args {
		if len(a) > 6 && a[:6] == "--wid=" {
			t.Fatalf("expected no --wid when WindowID is zero, got %q", a)
		}
	}

	var sawForceWindow bool
	for _, a := range args {
		if a == "--force-window=yes" {
			sawForceWindow = true
		}
	}
	if !sawForceWindow {
		t.Error("with no window to embed into, mpv must be told to open its own")
	}
}
