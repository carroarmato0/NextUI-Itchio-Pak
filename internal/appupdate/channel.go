package appupdate

// Channel is which releases the user wants to hear about.
type Channel string

const (
	Stable Channel = "stable" // releases with prerelease=false
	RC     Channel = "rc"     // every non-draft release, final ones included
	Off    Channel = "off"    // no request is made
)

// ResolveChannel turns config.json's update_channel into the channel in use.
// Empty (or unrecognised) means not chosen yet, and follows the running build:
// rc for a release candidate, stable otherwise. pin reports that "rc" should
// now be written, so a tester who installs the final release is not silently
// dropped to Stable. Stable is never pinned: a stable user who side-loads an
// rc has shown they want rcs, and gets them the same way (spec §1).
func ResolveChannel(stored string, running Version) (ch Channel, pin bool) {
	switch c := Channel(stored); c {
	case Stable, RC, Off:
		return c, false
	}
	if running.IsRC() {
		return RC, true
	}
	return Stable, false
}

// Next is the channel A cycles to on the Updates screen.
func (c Channel) Next() Channel {
	switch c {
	case Stable:
		return RC
	case RC:
		return Off
	default:
		return Stable
	}
}

// Label is the channel's name on screen.
func (c Channel) Label() string {
	switch c {
	case RC:
		return "Release candidates"
	case Off:
		return "Off"
	default:
		return "Stable"
	}
}
