package appupdate

import (
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/firmware"
	"github.com/carroarmato0/nextui-itchio-pak/internal/pakstore"
)

// Kind is the outcome of a decision.
type Kind int

const (
	Unknown   Kind = iota // nothing known yet, or the channel is Off
	UpToDate              // nothing newer than the running build
	Available             // Latest is newer than the running build
)

func (k Kind) String() string {
	switch k {
	case UpToDate:
		return "up-to-date"
	case Available:
		return "available"
	default:
		return "unknown"
	}
}

// Via is the route an available update takes.
type Via int

const (
	// ViaReleasePage: show the version and the release-page QR code.
	ViaReleasePage Via = iota
	// ViaPakStore: NextUI, Store-managed, and the Store will show it.
	ViaPakStore
	// ViaArchive: muOS, and the release has a .muxapp with a digest.
	ViaArchive
)

func (v Via) String() string {
	switch v {
	case ViaPakStore:
		return "pak-store"
	case ViaArchive:
		return "archive"
	default:
		return "release-page"
	}
}

// Verdict is what the app tells the user.
type Verdict struct {
	Kind    Kind
	Channel Channel
	Running Version
	Latest  *Release
	Via     Via
	// Ahead: UpToDate, and the running build is newer than Latest (an rc
	// tester on the Stable channel).
	Ahead bool
	// StoreOffers is a version the Pak Store may offer as an "update" that is
	// older than the running build (spec §2a). Shown on the Updates screen
	// only, never as a notice.
	StoreOffers string
}

// Inputs is everything Decide looks at.
type Inputs struct {
	Firmware firmware.Kind
	Running  Version
	Channel  Channel
	Store    pakstore.Result
	Latest   *Release
	PakJSON  *Release
}

// Decide is a pure function from the inputs to a Verdict. "Available" needs
// latest > running by the real ordering, whatever the Store row says: a
// side-loaded rc must never be told to "update" to an older stable.
func Decide(in Inputs) Verdict {
	out := Verdict{Channel: in.Channel, Running: in.Running, Latest: in.Latest, StoreOffers: storeOffers(in)}
	if in.Channel == Off || in.Latest == nil {
		return out
	}
	latest, ok := Parse(in.Latest.Tag)
	if !ok {
		return out
	}
	switch c := Compare(latest, in.Running); {
	case c <= 0:
		out.Kind, out.Ahead = UpToDate, c < 0
	default:
		out.Kind, out.Via = Available, via(in, latest)
	}
	return out
}

func via(in Inputs, latest Version) Via {
	switch in.Firmware {
	case firmware.KindNextUI:
		// The Store never offers a release candidate, and offers a final
		// release only when its row compares lower (its comparison thinks
		// v1.1.0-rc2 == v1.1.0: the Store-row trap).
		if in.Store.Status != pakstore.NotInstalled && !latest.IsRC() {
			// The Store offers what pak.json on main says, not GitHub's
			// latest: until main is bumped it would offer an older version,
			// so point at the release page instead.
			if in.PakJSON != nil {
				if pj, ok := Parse(in.PakJSON.Tag); ok && Compare(pj, latest) < 0 {
					return ViaReleasePage
				}
			}
			if in.Store.Status == pakstore.Unknown || StoreCompare(in.Store.Version, in.Latest.Tag) == -1 {
				return ViaPakStore
			}
		}
	case firmware.KindMuOS:
		if in.Latest.Asset != "" && in.Latest.Size > 0 && strings.HasPrefix(in.Latest.Digest, "sha256:") {
			return ViaArchive
		}
	}
	return ViaReleasePage
}

// storeOffers is the §2a downgrade trap: the Store compares its stale row with
// pak.json on main, not with what is on disk.
func storeOffers(in Inputs) string {
	if in.Firmware != firmware.KindNextUI || in.Store.Status != pakstore.Managed || in.PakJSON == nil {
		return ""
	}
	pj, ok := Parse(in.PakJSON.Tag)
	if !ok {
		return ""
	}
	if StoreCompare(in.Store.Version, in.PakJSON.Tag) == -1 && Compare(pj, in.Running) < 0 {
		return in.PakJSON.Tag
	}
	return ""
}

// ShouldNotify is the rule of spec §2: Available, and newer than the last
// version announced on this channel.
func ShouldNotify(v Verdict, notified string) bool {
	if v.Kind != Available || v.Latest == nil {
		return false
	}
	latest, ok := Parse(v.Latest.Tag)
	if !ok {
		return false
	}
	prev, ok := Parse(notified)
	return !ok || Compare(latest, prev) > 0
}
