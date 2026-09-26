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
}
