// Package netstate knows whether the app can reach the network. It classifies
// request errors and keeps one app-wide Online/Offline state that every HTTP
// client feeds. It has no SDL dependency, so it is fully unit-tested.
package netstate

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"syscall"
	"time"

	"golang.org/x/net/http2"
)

// Reason says why a request failed.
type Reason int

const (
	ReasonNone        Reason = iota // no error
	ReasonNoNetwork                 // no route: Wi-Fi off or not associated
	ReasonDNS                       // name lookup failed
	ReasonUnreachable               // refused, reset, timed out, cut off
	ReasonClock                     // certificate "expired" because the device clock is wrong
	ReasonIntercepted               // a network in the way: captive portal, proxy
	ReasonCanceled                  // the user gave up (Back); not a network problem
	ReasonOther                     // anything else, including HTTP statuses
)

func (r Reason) String() string {
	switch r {
	case ReasonNone:
		return "none"
	case ReasonNoNetwork:
		return "no-network"
	case ReasonDNS:
		return "dns"
	case ReasonUnreachable:
		return "unreachable"
	case ReasonClock:
		return "clock"
	case ReasonIntercepted:
		return "intercepted"
	case ReasonCanceled:
		return "canceled"
	default:
		return "other"
	}
}

// Offline reports whether r means the network, not the server, is the problem.
func (r Reason) Offline() bool {
	switch r {
	case ReasonNoNetwork, ReasonDNS, ReasonUnreachable, ReasonClock, ReasonIntercepted:
		return true
	}
	return false
}

// StatusError is an HTTP response the caller did not want. The network worked.
type StatusError struct {
	What string
	Code int
}

func (e *StatusError) Error() string { return fmt.Sprintf("%s: HTTP %d", e.What, e.Code) }

// buildDate is the commit date of this build, set at build time:
//
//	-X github.com/carroarmato0/nextui-itchio-pak/internal/netstate.buildDate=2026-09-26T10:00:00+02:00
var buildDate = ""

// clockFloor is used when buildDate is missing (dev builds). A device whose
// clock reads earlier than the build it is running has certainly lost the date.
var clockFloor = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

var now = time.Now

func earliestPlausible() time.Time {
	if t, err := time.Parse(time.RFC3339, buildDate); err == nil && t.After(clockFloor) {
		return t
	}
	return clockFloor
}

// Classify says why err happened. The order matters: a cancelled request can
// wrap a network error, and the user pressing Back must win.
func Classify(err error) Reason {
	if err == nil {
		return ReasonNone
	}
	if errors.Is(err, context.Canceled) {
		return ReasonCanceled
	}
	var invalid x509.CertificateInvalidError
	if errors.As(err, &invalid) && invalid.Reason == x509.Expired {
		// x509 reports "not yet valid" as Expired too. Only blame the clock
		// when it provably is wrong; otherwise something is presenting a bad
		// certificate, which is what captive portals do.
		if now().Before(earliestPlausible()) {
			return ReasonClock
		}
		return ReasonIntercepted
	}
	var unknownCA x509.UnknownAuthorityError
	var hostname x509.HostnameError
	if errors.As(err, &unknownCA) || errors.As(err, &hostname) {
		return ReasonIntercepted
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return ReasonDNS
	}
	if errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH) {
		return ReasonNoNetwork
	}
	// io.EOF directly inside a *url.Error means the connection closed before
	// any response arrived: the network. Anywhere else (decoding a body that
	// came back empty, reading a local file) it is a short read, not the
	// network, so it falls through to Other.
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err == io.EOF {
		return ReasonUnreachable
	}
	// EPIPE is a reset that lands while the request is still being written:
	// which of the two the client sees is a race, not a different failure.
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return ReasonUnreachable
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return ReasonUnreachable
	}
	if isHTTP2ConnFailure(err) {
		return ReasonUnreachable
	}
	return ReasonOther
}

// http2ConnFailures are the texts of x/net/http2's unexported errors for a
// connection that went away under a request. They have no type to match, so
// each chain link is compared whole; a bare "http2: " prefix would also catch
// protocol limits (header list too large) that are not the network.
var http2ConnFailures = map[string]bool{
	"http2: client connection lost":                               true,
	"http2: client conn is closed":                                true,
	"http2: client conn not usable":                               true,
	"http2: Transport received Server's graceful shutdown GOAWAY": true,
}

// isHTTP2ConnFailure reports whether err is the HTTP/2 layer losing the
// stream or connection: a reset stream, a GOAWAY that closed the connection,
// a connection-level error, or one of the transport's connection-lost errors.
// The response never arrived or was cut off, the same as a TCP reset.
func isHTTP2ConnFailure(err error) bool {
	var se http2.StreamError
	var ga http2.GoAwayError
	var ce http2.ConnectionError
	if errors.As(err, &se) || errors.As(err, &ga) || errors.As(err, &ce) {
		return true
	}
	return anyInChain(err, func(e error) bool { return http2ConnFailures[e.Error()] })
}

// anyInChain reports whether match holds for err or anything it wraps,
// following both single and multiple (errors.Join, %w %w) wrapping.
func anyInChain(err error, match func(error) bool) bool {
	if err == nil {
		return false
	}
	if match(err) {
		return true
	}
	switch u := err.(type) {
	case interface{ Unwrap() error }:
		return anyInChain(u.Unwrap(), match)
	case interface{ Unwrap() []error }:
		for _, e := range u.Unwrap() {
			if anyInChain(e, match) {
				return true
			}
		}
	}
	return false
}

// Detail is err's text for the log, without the request URL: the text of a
// failed request quotes the whole URL, and signed CDN URLs carry credentials.
func Detail(err error) string {
	if err == nil {
		return ""
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Op + ": " + ue.Err.Error()
	}
	return err.Error()
}
