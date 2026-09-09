package mpvipc

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// mpv decides a JSON number's type from its spelling: no fractional part means
// an integer, which its float properties reject outright. Every value must
// therefore carry a decimal point.
func TestFloatAlwaysMarshalsAsADouble(t *testing.T) {
	cases := []struct {
		name string
		in   Float
		want string
	}{
		{"whole number", 131, "131.0"},
		{"the volume maximum", 150, "150.0"},
		{"normal speed", 1, "1.0"},
		{"double speed", 2, "2.0"},
		{"eight times speed", 8, "8.0"},
		{"zero", 0, "0.0"},
		{"negative whole number", -30, "-30.0"},
		{"already fractional", 1.25, "1.25"},
		{"small fraction", 0.1, "0.1"},
		{"negative fraction", -0.5, "-0.5"},
		{"a position in seconds", 8309.237, "8309.237"},
		{"a large whole number", 1000000, "1000000.0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.in)
			if err != nil {
				t.Fatalf("marshalling %v: %v", float64(tc.in), err)
			}
			if string(raw) != tc.want {
				t.Errorf("Float(%v) marshalled as %s, want %s", float64(tc.in), raw, tc.want)
			}
			if !strings.Contains(string(raw), ".") {
				t.Errorf("Float(%v) marshalled as %s, which mpv reads as an integer and rejects",
					float64(tc.in), raw)
			}
		})
	}
}

// A bare float64 is what produced the bug; this pins the difference so the
// reason for the wrapper stays visible.
func TestPlainFloat64LosesTheDecimalPoint(t *testing.T) {
	raw, err := json.Marshal(float64(131))
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	if string(raw) != "131" {
		t.Skipf("Go now encodes float64(131) as %s; the wrapper may be unnecessary", raw)
	}
}

// Exponent notation would still be valid JSON, but it is unreadable in a log
// and depends on mpv's parser treating it as a double.
func TestFloatNeverUsesExponentNotation(t *testing.T) {
	for _, value := range []Float{1e21, 1e-9, 123456789012345678} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshalling %v: %v", float64(value), err)
		}
		if strings.ContainsAny(string(raw), "eE") {
			t.Errorf("Float(%v) marshalled as %s, which uses exponent notation", float64(value), raw)
		}
	}
}

// The whole point is what reaches mpv, so these check the bytes on the wire.
// Decoding the request and re-encoding it would erase the very distinction
// under test, because JSON decodes both 2 and 2.0 to the same float64.

func TestSetPropertyWithAFloatSendsADouble(t *testing.T) {
	c, f := startFakeMPV(t, nil, func(_ *fakeMPV, req request) string {
		return `{"error":"success","request_id":` + itoa(req.RequestID) + `}`
	})

	if err := c.SetProperty(testCtx(t), "volume", Float(150)); err != nil {
		t.Fatalf("SetProperty: %v", err)
	}

	got := f.lastRaw()
	if !strings.Contains(got, "150.0") {
		t.Errorf("mpv received %s, want the volume written as 150.0 rather than 150", got)
	}
}

func TestSetPropertyWithAWholeNumberSpeed(t *testing.T) {
	c, f := startFakeMPV(t, nil, func(_ *fakeMPV, req request) string {
		return `{"error":"success","request_id":` + itoa(req.RequestID) + `}`
	})

	for _, speed := range []Float{1, 2, 4, 8} {
		if err := c.SetProperty(testCtx(t), "speed", speed); err != nil {
			t.Fatalf("SetProperty(speed=%v): %v", float64(speed), err)
		}

		got := f.lastRaw()
		want := strconv.FormatFloat(float64(speed), 'f', -1, 64) + ".0"
		if !strings.Contains(got, want) {
			t.Errorf("speed %v was sent as %s, want it written as %s", float64(speed), got, want)
		}
	}
}

// A fractional value must not gain a spurious ".0".
func TestSetPropertyKeepsFractionsIntact(t *testing.T) {
	c, f := startFakeMPV(t, nil, func(_ *fakeMPV, req request) string {
		return `{"error":"success","request_id":` + itoa(req.RequestID) + `}`
	})

	if err := c.SetProperty(testCtx(t), "speed", Float(1.25)); err != nil {
		t.Fatalf("SetProperty: %v", err)
	}
	if got := f.lastRaw(); !strings.Contains(got, "1.25") || strings.Contains(got, "1.25.0") {
		t.Errorf("mpv received %s, want the speed written as 1.25", got)
	}
}
