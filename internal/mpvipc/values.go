package mpvipc

import (
	"strconv"
	"strings"
)

// Float wraps a value destined for one of mpv's floating-point properties.
//
// mpv's JSON parser decides a value's type from how it is written: a number
// with no fractional part becomes an integer node, one with a fractional part
// becomes a double. Its float properties — volume, speed, sub-delay,
// audio-delay — accept a double but reject an integer with "unsupported format
// for accessing property".
//
// Go's encoder writes float64(2) as "2", so setting the speed to exactly 2x,
// or the volume to exactly 130, would fail while 2.5 and 130.5 succeeded. This
// type forces the decimal point so every value is unambiguously a double.
type Float float64

// MarshalJSON writes the value with a fractional part always present.
func (f Float) MarshalJSON() ([]byte, error) {
	// 'f' rather than 'g': an exponent form ("1e+02") is valid JSON but reads
	// as a double to mpv only by accident of formatting, and is unreadable in
	// a log.
	s := strconv.FormatFloat(float64(f), 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return []byte(s), nil
}
