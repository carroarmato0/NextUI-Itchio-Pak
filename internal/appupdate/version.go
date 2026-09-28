// Package appupdate tells the user when a newer Itch-io exists: it checks
// GitHub, decides what to say, and on muOS saves the update for Archive
// Manager. It has no SDL dependency, so all of it is unit-tested headless.
package appupdate

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a release tag: vMAJOR.MINOR.PATCH with an optional -rcN.
type Version struct {
	Major, Minor, Patch int
	RC                  int // 0 for a final release
}

// Parse reads a release tag; the leading "v" is optional. Anything else —
// "dev", "1.1", "v1.1.0-beta1" — is not a version this app compares, and no
// update check runs for a build carrying it.
func Parse(s string) (Version, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	core, rc, hasRC := strings.Cut(s, "-rc")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	var n [3]int
	for i, p := range parts {
		v, ok := number(p)
		if !ok {
			return Version{}, false
		}
		n[i] = v
	}
	out := Version{Major: n[0], Minor: n[1], Patch: n[2]}
	if hasRC {
		r, ok := number(rc)
		if !ok || r == 0 {
			return Version{}, false
		}
		out.RC = r
	}
	return out, true
}

// number accepts plain decimal digits only; Atoi alone would also take "+1".
func number(s string) (int, bool) {
	if s == "" || len(s) > 6 {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// Compare orders versions: 1.1.0-rc1 < 1.1.0-rc2 < 1.1.0 < 1.1.1-rc1.
func Compare(a, b Version) int {
	for _, d := range [3][2]int{{a.Major, b.Major}, {a.Minor, b.Minor}, {a.Patch, b.Patch}} {
		if d[0] != d[1] {
			if d[0] < d[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case a.RC == b.RC:
		return 0
	case a.RC == 0: // a final release is newer than all of its candidates
		return 1
	case b.RC == 0:
		return -1
	case a.RC < b.RC:
		return -1
	default:
		return 1
	}
}

// IsRC reports whether v is a release candidate.
func (v Version) IsRC() bool { return v.RC > 0 }

func (v Version) String() string {
	s := fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.RC > 0 {
		s += fmt.Sprintf("-rc%d", v.RC)
	}
	return s
}

// StoreCompare is the Pak Store's compareVersions (LoveRetro/nextui-pak-store,
// state/helpers.go), ported line for line. It is used only to predict what the
// Store will show. It reads "0-rc1" as 0, so v1.1.0-rc1 equals v1.1.0: the
// Store can never tell a release candidate from its final release.
func StoreCompare(a, b string) int {
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")

	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")

	maxLen := len(partsA)
	if len(partsB) > maxLen {
		maxLen = len(partsB)
	}

	for i := 0; i < maxLen; i++ {
		var numA, numB int

		if i < len(partsA) {
			numA, _ = strconv.Atoi(partsA[i])
		}
		if i < len(partsB) {
			numB, _ = strconv.Atoi(partsB[i])
		}

		if numA < numB {
			return -1
		}
		if numA > numB {
			return 1
		}
	}

	return 0
}
