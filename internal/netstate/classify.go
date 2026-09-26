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
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return ReasonUnreachable
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return ReasonUnreachable
	}
	return ReasonOther
}

// detailOf is err's text for the log, without the request URL: the text of a
// failed request quotes the whole URL, and signed CDN URLs carry credentials.
func detailOf(err error) string {
	if err == nil {
		return ""
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Op + ": " + ue.Err.Error()
	}
	return err.Error()
}
