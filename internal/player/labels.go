package player

import (
	"fmt"
	"strings"
)

// labelSeparator joins a track's name to its technical description.
const labelSeparator = " — "

// imageSubtitleCodecs are subtitle formats stored as pictures. They can be
// displayed but never turned into a transcript without OCR, which is out of
// scope, so the UI needs to know which tracks these are up front.
var imageSubtitleCodecs = map[string]bool{
	"hdmv_pgs_subtitle": true,
	"pgs":               true,
	"dvd_subtitle":      true,
	"dvdsub":            true,
	"vobsub":            true,
	"dvb_subtitle":      true,
	"dvbsub":            true,
	"xsub":              true,
	"hdmv_text_subtitle": false, // text despite the "hdmv" prefix
}

// codecNames maps container codec identifiers to the names people recognise.
// Anything absent falls through to an upper-cased form of the raw identifier,
// which is still more readable than the original.
var codecNames = map[string]string{
	// Audio
	"aac": "AAC", "ac3": "AC3", "eac3": "E-AC3", "dts": "DTS",
	"truehd": "TrueHD", "mp3": "MP3", "mp2": "MP2", "opus": "Opus",
	"vorbis": "Vorbis", "flac": "FLAC", "alac": "ALAC", "wmav2": "WMA",
	"pcm_s16le": "PCM", "pcm_s24le": "PCM", "pcm_s32le": "PCM", "pcm_f32le": "PCM",

	// Subtitles
	"subrip": "SRT", "srt": "SRT", "ass": "ASS", "ssa": "SSA",
	"webvtt": "WebVTT", "vtt": "WebVTT", "mov_text": "MP4 Text",
	"text": "Text", "hdmv_pgs_subtitle": "PGS", "dvd_subtitle": "VobSub",
	"dvb_subtitle": "DVB", "xsub": "XSUB",

	// Video
	"h264": "H.264", "avc1": "H.264", "hevc": "H.265", "h265": "H.265",
	"av1": "AV1", "vp8": "VP8", "vp9": "VP9", "mpeg2video": "MPEG-2",
	"mpeg4": "MPEG-4", "vc1": "VC-1", "prores": "ProRes", "theora": "Theora",
}

// CodecName renders a codec identifier for display.
func CodecName(codec string) string {
	codec = strings.TrimSpace(strings.ToLower(codec))
	if codec == "" {
		return ""
	}
	if name, ok := codecNames[codec]; ok {
		return name
	}
	// PCM has many variants; collapse them rather than listing every one.
	if strings.HasPrefix(codec, "pcm_") {
		return "PCM"
	}
	return strings.ToUpper(codec)
}

// IsImageSubtitle reports whether a subtitle codec is picture-based.
func IsImageSubtitle(codec string) bool {
	codec = strings.TrimSpace(strings.ToLower(codec))
	if known, ok := imageSubtitleCodecs[codec]; ok {
		return known
	}
	return false
}

// ChannelLayoutName describes a channel count the way listeners think about it.
func ChannelLayoutName(channels int) string {
	switch {
	case channels <= 0:
		return ""
	case channels == 1:
		return "Mono"
	case channels == 2:
		return "Stereo"
	case channels == 6:
		return "5.1"
	case channels == 8:
		return "7.1"
	default:
		return fmt.Sprintf("%d ch", channels)
	}
}

// trackName builds the human part of a label from a track's language and title.
//
// Titles frequently repeat the language ("English", "English Commentary"), so
// the two are merged rather than concatenated blindly, which would produce
// "English English Commentary".
func trackName(language, title string) string {
	lang := LanguageName(language)
	title = strings.TrimSpace(title)

	switch {
	case title == "" && lang == "":
		return ""
	case title == "":
		return lang
	case lang == "":
		return title
	}

	// The title already carries the language, so it stands on its own.
	if strings.EqualFold(title, lang) || strings.HasPrefix(strings.ToLower(title), strings.ToLower(lang)) {
		return title
	}
	return lang + " " + title
}

// LabelForAudio builds a menu label such as "English — AAC Stereo" or
// "English Commentary — AC3 5.1".
func LabelForAudio(t Track) string {
	name := trackName(t.Language, t.Title)
	if name == "" {
		name = fmt.Sprintf("Audio track %d", t.ID)
	}

	detail := strings.TrimSpace(strings.Join(nonEmpty(
		CodecName(t.Codec),
		ChannelLayoutName(t.Channels),
	), " "))

	if detail == "" {
		return name
	}
	return name + labelSeparator + detail
}

// LabelForSubtitle builds a menu label such as "English — SRT" or
// "English — PGS (image)".
func LabelForSubtitle(t Track) string {
	name := trackName(t.Language, t.Title)
	if name == "" {
		if t.External {
			name = "External subtitle"
		} else {
			name = fmt.Sprintf("Subtitle track %d", t.ID)
		}
	}

	var parts []string
	if codec := CodecName(t.Codec); codec != "" {
		parts = append(parts, codec)
	}
	if t.ImageBased {
		// Stated plainly so the empty transcript panel is never a mystery.
		parts = append(parts, "(image)")
	}
	if t.External {
		parts = append(parts, "(external)")
	}

	if len(parts) == 0 {
		return name
	}
	return name + labelSeparator + strings.Join(parts, " ")
}

// LabelForVideo builds a label such as "H.264 1920x1080".
func LabelForVideo(t Track) string {
	name := trackName(t.Language, t.Title)

	var parts []string
	if codec := CodecName(t.Codec); codec != "" {
		parts = append(parts, codec)
	}
	if t.Width > 0 && t.Height > 0 {
		parts = append(parts, fmt.Sprintf("%dx%d", t.Width, t.Height))
	}
	detail := strings.Join(parts, " ")

	switch {
	case name == "" && detail == "":
		return fmt.Sprintf("Video track %d", t.ID)
	case name == "":
		return detail
	case detail == "":
		return name
	default:
		return name + labelSeparator + detail
	}
}

// Label dispatches to the right labeller for a track's type.
func Label(t Track) string {
	switch t.Type {
	case TrackAudio:
		return LabelForAudio(t)
	case TrackSubtitle:
		return LabelForSubtitle(t)
	case TrackVideo:
		return LabelForVideo(t)
	default:
		return fmt.Sprintf("Track %d", t.ID)
	}
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// ResolutionName gives the shorthand people use for a frame size.
func ResolutionName(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	// Classified by height, since ultrawide and 4:3 material of the same class
	// share a height but not a width.
	switch {
	case height >= 4320:
		return "8K"
	case height >= 2160:
		return "4K"
	case height >= 1440:
		return "1440p"
	case height >= 1080:
		return "1080p"
	case height >= 720:
		return "720p"
	case height >= 576:
		return "576p"
	case height >= 480:
		return "480p"
	default:
		return fmt.Sprintf("%dp", height)
	}
}
