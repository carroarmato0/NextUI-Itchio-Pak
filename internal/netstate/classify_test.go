package netstate

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	defer func(old func() time.Time, bd string) { now, buildDate = old, bd }(now, buildDate)
	buildDate = "2026-09-26T10:00:00Z"
	now = func() time.Time { return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC) }

	expired := &tls.CertificateVerificationError{Err: x509.CertificateInvalidError{Reason: x509.Expired}}
	cases := []struct {
		name string
		err  error
		want Reason
	}{
		{"nil", nil, ReasonNone},
		{"dns", &url.Error{Op: "Get", URL: "https://itch.io", Err: &net.DNSError{Err: "no such host", Name: "itch.io", IsNotFound: true}}, ReasonDNS},
		{"net unreachable", &url.Error{Op: "Get", Err: &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ENETUNREACH)}}, ReasonNoNetwork},
		{"host unreachable", &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.EHOSTUNREACH)}, ReasonNoNetwork},
		{"refused", &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}, ReasonUnreachable},
		{"reset", &net.OpError{Op: "read", Err: os.NewSyscallError("read", syscall.ECONNRESET)}, ReasonUnreachable},
		// io.EOF directly inside a *url.Error: the connection closed before
		// any response arrived. That is the network.
		{"eof, no response", &url.Error{Op: "Get", Err: io.EOF}, ReasonUnreachable},
		{"eof, no response, wrapped", fmt.Errorf("fetch data.json: %w", &url.Error{Op: "Get", Err: io.EOF}), ReasonUnreachable},
		// Anywhere else io.EOF is an empty or short read of a body or a local
		// file (a 200 with no body): not the network.
		{"eof, body decode", fmt.Errorf("decode data.json: %w", io.EOF), ReasonOther},
		{"eof, bare", io.EOF, ReasonOther},
		{"unexpected eof", fmt.Errorf("read stream: %w", io.ErrUnexpectedEOF), ReasonUnreachable},
		{"deadline", fmt.Errorf("no data: %w", os.ErrDeadlineExceeded), ReasonUnreachable},
		{"expired, plausible clock", expired, ReasonIntercepted},
		{"unknown authority", &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}, ReasonIntercepted},
		{"hostname", x509.HostnameError{Host: "itch.io"}, ReasonIntercepted},
		{"canceled", &url.Error{Op: "Get", Err: context.Canceled}, ReasonCanceled},
		{"status", &StatusError{What: "feed", Code: 503}, ReasonOther},
		{"plain", errors.New("not a valid ZIP"), ReasonOther},
	}
	for _, c := range cases {
		if got := Classify(c.err); got != c.want {
			t.Errorf("%s: Classify = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestClassify_eofWithNoResponseIsOffline(t *testing.T) {
	if !Classify(&url.Error{Op: "Get", URL: "https://itch.io/x", Err: io.EOF}).Offline() {
		t.Fatal("io.EOF directly inside a *url.Error (no response) must count as offline")
	}
}

func TestClassify_wrappedEOFIsNotOffline(t *testing.T) {
	if Classify(fmt.Errorf("decode data.json: %w", io.EOF)).Offline() {
		t.Fatal("a wrapped io.EOF must not count as offline")
	}
	if _, ok := Describe(fmt.Errorf("read zip: %w", io.EOF), "itch.io"); ok {
		t.Fatal("Describe must leave a local io.EOF to the screen's own text")
	}
}

func TestClassify_expiredWithClockBeforeBuildIsClock(t *testing.T) {
	defer func(old func() time.Time, bd string) { now, buildDate = old, bd }(now, buildDate)
	buildDate = "2026-09-26T10:00:00Z"
	now = func() time.Time { return time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC) }
	err := &tls.CertificateVerificationError{Err: x509.CertificateInvalidError{Reason: x509.Expired}}
	if got := Classify(err); got != ReasonClock {
		t.Fatalf("Classify = %v, want Clock", got)
	}
}

func TestClassify_emptyBuildDateUsesFloor(t *testing.T) {
	defer func(old func() time.Time, bd string) { now, buildDate = old, bd }(now, buildDate)
	buildDate = ""
	now = func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) }
	err := x509.CertificateInvalidError{Reason: x509.Expired}
	if got := Classify(err); got != ReasonClock {
		t.Fatalf("Classify = %v, want Clock (before the built-in floor)", got)
	}
}

// Real errors from the standard library, not hand-built ones.
func TestClassify_realRefusedAndTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	_, err = http.Get("http://" + addr + "/")
	if got := Classify(err); got != ReasonUnreachable {
		t.Errorf("refused: Classify(%v) = %v, want Unreachable", err, got)
	}

	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block }))
	defer srv.Close()
	defer close(block)
	c := &http.Client{Timeout: 50 * time.Millisecond}
	_, err = c.Get(srv.URL)
	if got := Classify(err); got != ReasonUnreachable {
		t.Errorf("timeout: Classify(%v) = %v, want Unreachable", err, got)
	}
}

func TestReasonOffline(t *testing.T) {
	for _, r := range []Reason{ReasonNoNetwork, ReasonDNS, ReasonUnreachable, ReasonClock, ReasonIntercepted} {
		if !r.Offline() {
			t.Errorf("%v.Offline() = false", r)
		}
	}
	for _, r := range []Reason{ReasonNone, ReasonCanceled, ReasonOther} {
		if r.Offline() {
			t.Errorf("%v.Offline() = true", r)
		}
	}
}

func TestDetailOf_stripsURL(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "https://cdn.example/f.gb?sig=SECRET", Err: io.ErrUnexpectedEOF}
	if got := Detail(err); got != "Get: unexpected EOF" {
		t.Fatalf("Detail = %q", got)
	}
}
