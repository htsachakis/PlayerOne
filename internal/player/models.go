package player

// Track types, matching mpv's own vocabulary so no translation layer is needed
// when reading track-list.
const (
	TrackVideo    = "video"
	TrackAudio    = "audio"
	TrackSubtitle = "sub"
)

// NoTrack is the track ID meaning "none selected". mpv uses the string "no" for
// this on the sid/aid properties; zero is the equivalent on our side because
// mpv's real track IDs start at 1.
const NoTrack = 0

// Chapter is one navigable point in the media.
type Chapter struct {
	Index int     `json:"index"`
	Title string  `json:"title"`
	Start float64 `json:"start"`
}

// Track is one selectable stream.
type Track struct {
	ID       int    `json:"id"`
	Type     string `json:"type"`
	Language string `json:"language"`
	Title    string `json:"title"`
	Codec    string `json:"codec"`
	Selected bool   `json:"selected"`
	Default  bool   `json:"default"`

	// External is true for a subtitle loaded from a separate file.
	External         bool   `json:"external"`
	ExternalFilename string `json:"externalFilename"`

	// FFIndex is the stream's index within the container as ffmpeg numbers it.
	// It is what makes `ffmpeg -map 0:N` line up with the track the user picked,
	// and is -1 when mpv does not report one (which is the case for external
	// subtitle files, where the filename is used instead).
	FFIndex int `json:"ffIndex"`

	Channels   int     `json:"channels"`
	SampleRate int     `json:"sampleRate"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	FPS        float64 `json:"fps"`

	// ImageBased marks a subtitle track that is a picture rather than text, so
	// the UI can explain why no transcript is available instead of showing an
	// empty panel.
	ImageBased bool `json:"imageBased"`

	// Label is the human-readable name shown in menus, for example
	// "English - AAC Stereo".
	Label string `json:"label"`
}

// PlaybackState is the authoritative snapshot of the player, pushed to the
// frontend. The frontend keeps exactly one copy of this.
type PlaybackState struct {
	Position float64 `json:"position"`
	Duration float64 `json:"duration"`
	Paused   bool    `json:"paused"`
	Muted    bool    `json:"muted"`
	Volume   float64 `json:"volume"`
	Speed    float64 `json:"speed"`

	// Reverse reports whether mpv is playing backwards.
	Reverse bool `json:"reverse"`

	ChapterIndex int `json:"chapterIndex"`

	// FileLoaded distinguishes "nothing open" from "open and paused at zero".
	FileLoaded bool   `json:"fileLoaded"`
	Path       string `json:"path"`
	Title      string `json:"title"`

	Idle    bool `json:"idle"`
	Seeking bool `json:"seeking"`
	EOF     bool `json:"eof"`

	SubtitleID int `json:"subtitleId"`
	AudioID    int `json:"audioId"`

	SubtitleDelay float64 `json:"subtitleDelay"`
	AudioDelay    float64 `json:"audioDelay"`

	// HWDec names the active hardware decoder, or "no" when decoding on the CPU.
	// Surfaced in the Info tab because it is the first thing worth checking when
	// playback stutters.
	HWDec string `json:"hwdec"`
}

// VideoInfo describes the active video stream.
type VideoInfo struct {
	Codec       string  `json:"codec"`
	CodecLong   string  `json:"codecLong"`
	Profile     string  `json:"profile"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	FPS         float64 `json:"fps"`
	Bitrate     int64   `json:"bitrate"`
	PixelFormat string  `json:"pixelFormat"`
}

// AudioInfo describes one audio stream for the Info tab.
type AudioInfo struct {
	ID            int    `json:"id"`
	Codec         string `json:"codec"`
	CodecLong     string `json:"codecLong"`
	Language      string `json:"language"`
	Title         string `json:"title"`
	Channels      int    `json:"channels"`
	ChannelLayout string `json:"channelLayout"`
	SampleRate    int    `json:"sampleRate"`
	Bitrate       int64  `json:"bitrate"`
	Label         string `json:"label"`
}

// SubtitleInfo describes one subtitle stream for the Info tab.
type SubtitleInfo struct {
	ID         int    `json:"id"`
	Codec      string `json:"codec"`
	Language   string `json:"language"`
	Title      string `json:"title"`
	External   bool   `json:"external"`
	ImageBased bool   `json:"imageBased"`
	Label      string `json:"label"`
}

// MediaInfo is everything known about the loaded file.
type MediaInfo struct {
	Path      string `json:"path"`
	Filename  string `json:"filename"`
	Directory string `json:"directory"`
	FileSize  int64  `json:"fileSize"`

	Duration      float64 `json:"duration"`
	Container     string  `json:"container"`
	ContainerLong string  `json:"containerLong"`
	Title         string  `json:"title"`
	CreationDate  string  `json:"creationDate"`
	OverallBitrate int64  `json:"overallBitrate"`

	Video     VideoInfo      `json:"video"`
	HasVideo  bool           `json:"hasVideo"`
	Audio     []AudioInfo    `json:"audio"`
	Subtitles []SubtitleInfo `json:"subtitles"`
	Chapters  []Chapter      `json:"chapters"`
	Tracks    []Track        `json:"tracks"`

	VideoTrackCount    int `json:"videoTrackCount"`
	AudioTrackCount    int `json:"audioTrackCount"`
	SubtitleTrackCount int `json:"subtitleTrackCount"`

	// ProbeError records why ffprobe could not add its detail. Playback is
	// unaffected, so this is informational rather than an error path.
	ProbeError string `json:"probeError"`
}

// ChapterAt returns the index of the chapter containing a position, or -1.
//
// Chapters are assumed to be in ascending order, which is how both mpv and
// Matroska provide them.
func ChapterAt(chapters []Chapter, position float64) int {
	found := -1
	for i, c := range chapters {
		if position+chapterSeekEpsilon < c.Start {
			break
		}
		found = i
	}
	return found
}

// chapterSeekEpsilon absorbs the small overshoot between asking mpv to seek to a
// chapter start and the position it reports back. Without it, seeking to a
// chapter can briefly highlight the previous one.
const chapterSeekEpsilon = 0.05
