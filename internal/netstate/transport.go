package netstate

import "net/http"

type transport struct{ next http.RoundTripper }

// Transport wraps next so every request reports its outcome: any response,
// whatever its status, means the network works; an error is classified.
func Transport(next http.RoundTripper) http.RoundTripper { return transport{next} }

func (t transport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	Report(err)
	return resp, err
}
