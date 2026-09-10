package version

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"1.1.0", "1.0.9", 1},
		{"2.0.0", "1.99.99", 1},

		// A leading v is how tags are written, and must not affect the order.
		{"v1.0.1", "1.0.0", 1},
		{"1.0.1", "v1.0.1", 0},
		{"V1.0.1", "v1.0.1", 0},

		// Numeric, not lexical: 10 is greater than 9.
		{"1.10.0", "1.9.0", 1},
		{"1.0.10", "1.0.9", 1},

		// A missing component counts as zero.
		{"1.2", "1.2.0", 0},
		{"1.2", "1.2.1", -1},
		{"1", "1.0.0", 0},

		// A pre-release comes before the release it leads to.
		{"1.0.0-beta", "1.0.0", -1},
		{"1.0.0", "1.0.0-beta", 1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-rc1", "1.0.0-rc2", -1},

		// Build metadata is ignored entirely.
		{"1.0.0+build9", "1.0.0", 0},

		{"", "", 0},
		{"", "1.0.0", -1},

		// Whitespace around a tag read from an API response.
		{" 1.0.1 ", "1.0.0", 1},
	}

	for _, tc := range cases {
		if got := Compare(tc.a, tc.b); got != tc.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCompareIsAntisymmetric(t *testing.T) {
	pairs := [][2]string{
		{"1.0.0", "1.0.1"},
		{"1.0.0-beta", "1.0.0"},
		{"2.1.0", "2.0.9"},
		{"1.0", "1.0.0"},
	}

	for _, p := range pairs {
		forward := Compare(p[0], p[1])
		backward := Compare(p[1], p[0])
		if forward != -backward {
			t.Errorf("Compare(%q,%q)=%d but Compare(%q,%q)=%d",
				p[0], p[1], forward, p[1], p[0], backward)
		}
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		name              string
		current, candidate string
		want              bool
	}{
		{"a later release", "1.0.0", "1.0.1", true},
		{"the same release", "1.0.0", "1.0.0", false},
		{"an older release", "1.0.1", "1.0.0", false},
		{"a tag with a v prefix", "1.0.0", "v1.1.0", true},
		{"a pre-release of the same version", "1.0.0", "1.0.0-beta", false},

		// The important one: a working tree must never be "updated" to a
		// published release, which would discard the changes being made.
		{"a development build", "dev", "1.0.0", false},
		{"an empty version", "", "1.0.0", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsNewer(tc.current, tc.candidate); got != tc.want {
				t.Errorf("IsNewer(%q, %q) = %v, want %v", tc.current, tc.candidate, got, tc.want)
			}
		})
	}
}

func TestIsReleaseAndString(t *testing.T) {
	original := Version
	originalCommit := Commit
	t.Cleanup(func() {
		Version, Commit = original, originalCommit
	})

	Version, Commit = "dev", ""
	if IsRelease() {
		t.Error("a dev build should not report itself as a release")
	}
	if String() != "development build" {
		t.Errorf("String() = %q", String())
	}

	Version, Commit = "1.2.3", "abcdef1234567890"
	if !IsRelease() {
		t.Error("1.2.3 should be a release")
	}
	if got := String(); got != "1.2.3 (abcdef1)" {
		t.Errorf("String() = %q, want the version with a short commit", got)
	}
}

func TestDetailIncludesTheBuildDate(t *testing.T) {
	originalVersion, originalCommit, originalDate := Version, Commit, Date
	t.Cleanup(func() {
		Version, Commit, Date = originalVersion, originalCommit, originalDate
	})

	Version, Commit, Date = "1.2.3", "", "2026-09-10"
	if got := Detail(); got != "1.2.3, built 2026-09-10" {
		t.Errorf("Detail() = %q", got)
	}
}
