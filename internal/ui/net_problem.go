package ui

import (
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
	return netstate.Detail(err)
}

// signInFailureText is the title and hint of the sign-in screen's failure
// state. Connection problems get netstate's words — a captive portal or an
// itch.io outage is not a Wi-Fi problem. A non-nil error that isn't a
// connection problem (a 401 from the profile check, a decode error) gets its
// own honest fallback instead of blaming Wi-Fi for something Wi-Fi didn't
// cause. Only a nil error — s.failErr is always set to the error that caused
// signInFailed, so this is not reachable from the running screen — keeps the
// old "can't reach" wording, since there is truly nothing to go on.
func signInFailureText(err error) (title, hint string) {
	if err == nil {
		return "Can't reach itch.io", "Check that Wi-Fi is on and connected, then try again."
	}
	if m, ok := netstate.Describe(err, "itch.io"); ok {
		return m.Title, m.Hint
	}
	return "Sign-in didn't work", "Try again. If it keeps failing, sign in again from Settings → Account."
}
