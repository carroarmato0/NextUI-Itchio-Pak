package itchio_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

// outageHTML is what an itch.io outage page looks like. None of it may reach
// the screen: the error must say "itch.io is having problems" and no more.
const outageHTML = "<!DOCTYPE html><html><body><h1>503 Service Unavailable</h1></body></html>"

func outageServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(outageHTML))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// assertOutage checks that err is a 503 StatusError, that Describe turns it
// into the spec's "having problems" message, and that its text carries
// neither the server's HTML nor the request URL.
func assertOutage(t *testing.T, name string, err error, srvURL string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected an error for HTTP 503", name)
	}
	var se *netstate.StatusError
	if !errors.As(err, &se) || se.Code != http.StatusServiceUnavailable {
		t.Errorf("%s: err = %v (%T), want a *netstate.StatusError with Code 503", name, err, err)
	}
	msg, ok := netstate.Describe(err, "itch.io")
	if !ok || msg.Title != "itch.io is having problems" {
		t.Errorf("%s: Describe = %+v, %v; want \"itch.io is having problems\"", name, msg, ok)
	}
	if s := err.Error(); strings.Contains(s, "<") || strings.Contains(s, srvURL) {
		t.Errorf("%s: error text leaks HTML or the URL: %q", name, s)
	}
}

func TestFetchGameDetail_503IsStatusError(t *testing.T) {
	defer netstate.ResetForTest()
	srv := outageServer(t)
	_, err := itchio.NewClientWithBase(srv.URL).FetchGameDetail(srv.URL + "/game")
	assertOutage(t, "FetchGameDetail", err, srv.URL)
}

func TestFetchUploads_503IsStatusError(t *testing.T) {
	defer netstate.ResetForTest()
	srv := outageServer(t)
	_, err := itchio.NewClientWithBase(srv.URL).FetchUploads(srv.URL + "/game")
	assertOutage(t, "FetchUploads", err, srv.URL)
}

func TestParseDownloadPage_503IsStatusError(t *testing.T) {
	defer netstate.ResetForTest()
	srv := outageServer(t)
	_, err := itchio.NewClientWithBase(srv.URL).ParseDownloadPage(srv.URL + "/download/abc")
	assertOutage(t, "ParseDownloadPage", err, srv.URL)
}

func TestResolveFreeURL_503IsStatusError(t *testing.T) {
	defer netstate.ResetForTest()
	srv := outageServer(t)
	upload := itchio.Upload{
		Filename: "game.gb",
		UploadID: "999",
		URL:      srv.URL + "/author/game/file/999?key=eyJpZCI6NDJ9.SIG&csrf=TOKEN",
	}
	_, err := itchio.NewClientWithBase(srv.URL).ResolveFreeURL(upload)
	assertOutage(t, "ResolveFreeURL", err, srv.URL)
}

// A 4xx from the resolver must not carry its body to the screen either.
func TestResolveFreeURL_errorBodyNotInError(t *testing.T) {
	defer netstate.ResetForTest()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(outageHTML))
	}))
	defer srv.Close()
	upload := itchio.Upload{Filename: "game.gb", UploadID: "1", URL: srv.URL + "/a/g/file/1?key=k&csrf=c"}
	_, err := itchio.NewClientWithBase(srv.URL).ResolveFreeURL(upload)
	if err == nil || strings.Contains(err.Error(), "<") {
		t.Fatalf("err = %v; want an error without the response body", err)
	}
}

func TestFetchUploadsForKey_503IsStatusError(t *testing.T) {
	defer netstate.ResetForTest()
	srv := outageServer(t)
	_, err := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL).FetchUploadsForKey("k", "1", "2")
	assertOutage(t, "FetchUploadsForKey", err, srv.URL)
}

func TestCreateDownloadSession_503IsStatusError(t *testing.T) {
	defer netstate.ResetForTest()
	srv := outageServer(t)
	_, err := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL).CreateDownloadSession("k", "1", "2")
	assertOutage(t, "CreateDownloadSession", err, srv.URL)
}

func TestResolveAuthURL_503IsStatusError(t *testing.T) {
	defer netstate.ResetForTest()
	srv := outageServer(t)
	_, err := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL).ResolveAuthURL("k", "1", roms.NewDownloadSession("1", "2"))
	assertOutage(t, "ResolveAuthURL", err, srv.URL)
}
