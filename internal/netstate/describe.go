package netstate

import "errors"

// Message is what the user is told about a failed request.
type Message struct {
	Title string
	Hint  string
}

// Describe turns err into words for the screen. service names who could not be
// reached ("itch.io", "GitHub"). It returns false when err is not a connection
// problem: the caller's own text ("not a valid ZIP") is then the better message.
func Describe(err error, service string) (Message, bool) {
	r := Classify(err)
	if r.Offline() {
		// A DNS failure with no route is really "Wi-Fi is off"; the monitor
		// knows that, the error alone does not.
		if cur := Current(); cur.Status == StatusOffline && cur.Reason == ReasonNoNetwork {
			r = ReasonNoNetwork
		}
	}
	switch r {
	case ReasonNoNetwork:
		return Message{"No Wi-Fi connection", "Turn on Wi-Fi in the system settings."}, true
	case ReasonDNS, ReasonUnreachable:
		return Message{"Can't reach " + service, "Check your Wi-Fi connection, then try again."}, true
	case ReasonClock:
		return Message{"The date and time are wrong", "Secure connections fail until the clock is set."}, true
	case ReasonIntercepted:
		return Message{"This network is blocking the connection", "Public Wi-Fi may need you to sign in on a phone or computer first."}, true
	}
	var se *StatusError
	if errors.As(err, &se) && se.Code >= 500 {
		return Message{service + " is having problems", "Try again in a few minutes."}, true
	}
	return Message{}, false
}
