package ui

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

// The sign-in failure screen says what went wrong, like every other screen,
// instead of telling a captive-portal or outage user to check their Wi-Fi.
func TestSignInFailureText(t *testing.T) {
	netstate.ResetForTest()
	defer netstate.ResetForTest()
	cases := []struct {
		name        string
		err         error
		title, hint string
	}{
		{"dns", &url.Error{Op: "Post", URL: "https://itch.io/oauth", Err: &net.DNSError{Err: "no such host", Name: "itch.io", IsNotFound: true}},
			"Can't reach itch.io", "Check your Wi-Fi connection, then try again."},
		{"captive portal", &url.Error{Op: "Post", URL: "https://itch.io/oauth", Err: &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}},
			"This network is blocking the connection", "Public Wi-Fi may need you to sign in on a phone or computer first."},
		{"outage", &netstate.StatusError{What: "start sign-in", Code: 503},
			"itch.io is having problems", "Try again in a few minutes."},
		{"unclassified decode error", errors.New("decode profile: unexpected token"),
			"Sign-in didn't work", "Try again. If it keeps failing, sign in again from Settings → Account."},
		{"401 from the profile check", fmt.Errorf("%w (HTTP %d)", itchio.ErrTokenRejected, 401),
			"Sign-in didn't work", "Try again. If it keeps failing, sign in again from Settings → Account."},
		{"no error recorded", nil,
			"Sign-in didn't work", "Try again. If it keeps failing, sign in again from Settings → Account."},
	}
	for _, c := range cases {
		title, hint := signInFailureText(c.err)
		if title != c.title || hint != c.hint {
			t.Errorf("%s: got %q / %q, want %q / %q", c.name, title, hint, c.title, c.hint)
		}
	}
}
