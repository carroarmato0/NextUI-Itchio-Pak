package itchio

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// itch.io lifted its Cloudflare block on the app; a 403 is an ordinary HTTP
// error now and the log must not claim otherwise.
func TestFetchTotalGames_403LogsNoCloudflare(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	c := NewClient()
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})
	_, err := c.FetchTotalGames()
	if !errors.Is(err, ErrCloudflareBlocked) {
		t.Fatalf("err = %v, want ErrCloudflareBlocked", err)
	}
	if !strings.Contains(buf.String(), "total-games HTTP 403") {
		t.Fatalf("403 not logged:\n%s", buf.String())
	}
	if strings.Contains(strings.ToLower(buf.String()), "cloudflare") {
		t.Errorf("log still blames Cloudflare:\n%s", buf.String())
	}
}
