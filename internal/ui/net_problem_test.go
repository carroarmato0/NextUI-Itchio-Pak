package ui

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"testing"
)

func TestProblemText(t *testing.T) {
	dns := &url.Error{Op: "Get", URL: "https://itch.io/games", Err: &net.DNSError{Err: "no such host", Name: "itch.io", IsNotFound: true}}
	got := problemText(dns)
	if got != "Can't reach itch.io. Check your Wi-Fi connection, then try again." {
		t.Fatalf("problemText(dns) = %q", got)
	}
	if strings.Contains(got, "https://") {
		t.Fatal("URL on screen")
	}
	own := errors.New("no downloadable files found for this game")
	if problemText(own) != own.Error() {
		t.Fatal("a non-network error lost its own text")
	}

	// An unclassified *url.Error (redirect loop, unusual TLS failure — not
	// something netstate.Describe recognizes) must still lose its URL: a
	// signed CDN URL carries credentials in the query string.
	unclassified := &url.Error{Op: "Get", URL: "https://cdn.itch.io/upload?token=secret", Err: errors.New("stopped after 10 redirects")}
	got = problemText(unclassified)
	if strings.Contains(got, "https://") {
		t.Fatal("URL on screen for an unclassified *url.Error")
	}
	if strings.Contains(got, "token=secret") {
		t.Fatal("credential leaked onto screen")
	}
	if !strings.Contains(got, "stopped after 10 redirects") {
		t.Fatalf("problemText(unclassified) = %q, lost its own text", got)
	}
}
