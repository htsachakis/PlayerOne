// Package version reports which build of PlayerOne is running, and compares
// version strings so the updater can tell newer from older.
package version

import (
	"fmt"
	"strconv"
	"strings"
)

// These are set at build time with -ldflags -X. A build without them - `go
// build` during development - reports itself as a development build rather
// than pretending to be a release, which matters because the updater must not
// offer to "update" a working tree to a published release.
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

// IsRelease reports whether this build carries a real version number.
func IsRelease() bool {
	return Version != "" && Version != "dev"
}

// String renders the version for display.
func String() string {
	if !IsRelease() {
		if Commit != "" {
			return "development build (" + shortCommit() + ")"
		}
		return "development build"
	}

	out := Version
	if Commit != "" {
		out += " (" + shortCommit() + ")"
	}
	return out
}

func shortCommit() string {
	if len(Commit) > 7 {
		return Commit[:7]
	}
	return Commit
}

// Compare orders two version strings, returning -1, 0 or 1 for a < b, a == b
// and a > b.
//
// This is a deliberately small subset of semantic versioning: numeric parts
// compared numerically, a leading "v" ignored, and a pre-release suffix ranked
// below the same version without one. It is enough to answer "is the release on
// GitHub newer than what is running", which is all the updater asks.
func Compare(a, b string) int {
	aNums, aPre := split(a)
	bNums, bPre := split(b)

	for i := 0; i < len(aNums) || i < len(bNums); i++ {
		// A missing component counts as zero, so 1.2 and 1.2.0 are equal.
		x, y := at(aNums, i), at(bNums, i)
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}

	// 1.0.0-beta precedes 1.0.0; two pre-releases fall back to text order,
	// which is right for the usual beta/rc naming.
	switch {
	case aPre == "" && bPre == "":
		return 0
	case aPre == "":
		return 1
	case bPre == "":
		return -1
	case aPre < bPre:
		return -1
	case aPre > bPre:
		return 1
	default:
		return 0
	}
}

// IsNewer reports whether candidate is a later version than current.
//
// A development build is never considered older than anything: offering to
// replace a working tree with a published release would throw away exactly the
// changes being worked on.
func IsNewer(current, candidate string) bool {
	if current == "" || current == "dev" {
		return false
	}
	return Compare(candidate, current) > 0
}

// split separates the numeric components from any pre-release suffix.
func split(v string) ([]int, string) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")

	// Build metadata never affects precedence.
	if plus := strings.IndexByte(v, '+'); plus >= 0 {
		v = v[:plus]
	}

	pre := ""
	if dash := strings.IndexByte(v, '-'); dash >= 0 {
		pre = v[dash+1:]
		v = v[:dash]
	}

	parts := strings.Split(v, ".")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			// A component that is not a number ends the comparable prefix;
			// treating the rest as a pre-release is the safest reading.
			if pre == "" {
				pre = p
			}
			break
		}
		nums = append(nums, n)
	}
	return nums, pre
}

func at(nums []int, i int) int {
	if i < len(nums) {
		return nums[i]
	}
	return 0
}

// Detail renders version, commit and build date for the Info tab.
func Detail() string {
	if !IsRelease() {
		return String()
	}
	if Date == "" {
		return String()
	}
	return fmt.Sprintf("%s, built %s", String(), Date)
}
