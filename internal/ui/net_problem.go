package ui

import (
	"errors"
	"net/url"

	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

// problemText is what a screen shows for err. Connection problems get plain
// words; anything else keeps its own text, which is usually already written
// for the user ("no downloadable files found for this game"). The raw error is
// logged where it happens, never drawn.
func problemText(err error) string {
	if m, ok := netstate.Describe(err, "itch.io"); ok {
		return m.Title + ". " + m.Hint
	}
	return withoutURL(err)
}

// withoutURL is err's text with any request URL stripped: *url.Error quotes
// the whole URL, and a signed CDN URL carries credentials in the query
// string. An error that is not a *url.Error keeps its own text unchanged.
func withoutURL(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Op + ": " + ue.Err.Error()
	}
	return err.Error()
}
