package timefmt

import "testing"

func TestFormat(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want string
	}{
		{"zero", 0, "0:00"},
		{"sub-minute", 5, "0:05"},
		{"pads seconds", 62, "1:02"},
		{"truncates fraction", 342.9, "5:42"},
		{"just under an hour", 3599, "59:59"},
		{"exactly an hour promotes", 3600, "1:00:00"},
		{"hours pad minutes and seconds", 3942, "1:05:42"},
		{"multi-hour", 8309.237, "2:18:29"},
		{"double-digit hours", 36000, "10:00:00"},
		{"negative clamps", -12, "0:00"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Format(tc.in); got != tc.want {
				t.Errorf("Format(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormatPadded(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "00:00"},
		{5, "00:05"},
		{180, "03:00"},
		{257, "04:17"},
		{3599, "59:59"},
		{3600, "1:00:00"},
		{8309, "2:18:29"},
		{-1, "00:00"},
	}

	for _, tc := range cases {
		if got := FormatPadded(tc.in); got != tc.want {
			t.Errorf("FormatPadded(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// NaN must not produce "NaN:NaN" or panic on the int64 conversion.
func TestFormatRejectsNaN(t *testing.T) {
	nan := zeroDiv()
	if got := Format(nan); got != "0:00" {
		t.Errorf("Format(NaN) = %q, want %q", got, "0:00")
	}
	if got := FormatPadded(nan); got != "00:00" {
		t.Errorf("FormatPadded(NaN) = %q, want %q", got, "00:00")
	}
}

func zeroDiv() float64 {
	zero := 0.0
	return zero / zero
}

func TestFormatClock(t *testing.T) {
	tests := []struct {
		seconds float64
		want    string
	}{
		{0, "00:00:00"},
		{-5, "00:00:00"},
		{42, "00:00:42"},
		{754, "00:12:34"},
		{3723, "01:02:03"},
		{360000, "100:00:00"},
	}

	for _, tc := range tests {
		if got := FormatClock(tc.seconds); got != tc.want {
			t.Errorf("FormatClock(%v) = %q, want %q", tc.seconds, got, tc.want)
		}
	}
}
