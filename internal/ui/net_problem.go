package ui

import "github.com/carroarmato0/nextui-itchio-pak/internal/netstate"

// problemText is what a screen shows for err. Connection problems get plain
// words; anything else keeps its own text, which is usually already written
// for the user ("no downloadable files found for this game"). The raw error is
// logged where it happens, never drawn.
func problemText(err error) string {
	if m, ok := netstate.Describe(err, "itch.io"); ok {
		return m.Title + ". " + m.Hint
	}
	return err.Error()
}
