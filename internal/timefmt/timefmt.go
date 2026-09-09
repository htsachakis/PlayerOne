// Package timefmt holds the application's single timestamp formatter.
//
// Having exactly one implementation is the point: chapters, the transcript, the
// seek bar and the resume prompt must never disagree about how a time looks.
package timefmt

import "fmt"

// Format renders a duration in seconds as a human timestamp.
//
// Under an hour it uses M:SS (5:42); an hour or more promotes to H:MM:SS
// (1:05:42). Negative and non-finite inputs clamp to zero rather than producing
// nonsense like "-1:-5".
func Format(seconds float64) string {
	if !(seconds > 0) { // also catches NaN
		seconds = 0
	}

	total := int64(seconds)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60

	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// FormatPadded is Format with a zero-padded minutes field (05:42 / 1:05:42).
// Chapter and transcript lists use it so timestamps form a straight column.
func FormatPadded(seconds float64) string {
	if !(seconds > 0) {
		seconds = 0
	}

	total := int64(seconds)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60

	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}
