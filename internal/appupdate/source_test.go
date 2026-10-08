package appupdate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../testdata/appupdate/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testSource(url string) *Source {
	s := NewSource("NextUI-Itchio-Pak/1.1.0 (+https://github.com/carroarmato0/NextUI-Itchio-Pak)")
	s.ReleasesURL, s.PakJSONURL = url+"/releases", url+"/pak.json"
	return s
}

func TestReleases_picksPerChannel(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("ETag", `W/"r1"`)
		w.Write(fixture(t, "releases.json"))
	}))
	defer srv.Close()

	res, err := testSource(srv.URL).Releases(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.RC == nil || res.RC.Tag != "v1.1.0-rc3" {
		t.Fatalf("RC = %+v, want v1.1.0-rc3 (the draft v1.2.0 must be ignored)", res.RC)
	}
	if res.RC.Asset != "https://example.invalid/v1.1.0-rc3.muxapp" || res.RC.Digest != "sha256:cc" || res.RC.Size != 12819887 {
		t.Fatalf("RC asset = %+v, want the .muxapp, not the pak zip", res.RC)
	}
	if res.Stable == nil || res.Stable.Tag != "v1.0.25" || res.Stable.Digest != "sha256:ff" {
		t.Fatalf("Stable = %+v, want v1.0.25", res.Stable)
	}
	if res.ETag != `W/"r1"` {
		t.Fatalf("ETag = %q", res.ETag)
	}
	if strings.Contains(gotUA, "tg5040") || !strings.HasPrefix(gotUA, "NextUI-Itchio-Pak/") {
		t.Fatalf("User-Agent = %q: must be the short form, no device details", gotUA)
	}
}

func TestReleases_finalBeatsItsCandidatesOnRC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"tag_name":"v1.1.0-rc4","prerelease":true},{"tag_name":"v1.1.0","prerelease":false}]`))
	}))
	defer srv.Close()
	res, err := testSource(srv.URL).Releases(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.RC.Tag != "v1.1.0" || res.Stable.Tag != "v1.1.0" {
		t.Fatalf("RC=%s Stable=%s; an RC tester must also be told about the final release", res.RC.Tag, res.Stable.Tag)
	}
	if res.RC.URL != "https://github.com/carroarmato0/NextUI-Itchio-Pak/releases/tag/v1.1.0" {
		t.Fatalf("missing html_url must fall back to the tag page, got %q", res.RC.URL)
	}
}

func TestReleases_noStableOnPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"tag_name":"v1.1.0-rc4","prerelease":true}]`))
	}))
	defer srv.Close()
	res, _ := testSource(srv.URL).Releases(context.Background(), "")
	if res.Stable != nil {
		t.Fatalf("Stable = %+v, want nil so the caller keeps its cached one", res.Stable)
	}
}

func TestReleases_notModified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != `W/"r1"` {
			t.Errorf("If-None-Match = %q", r.Header.Get("If-None-Match"))
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()
	res, err := testSource(srv.URL).Releases(context.Background(), `W/"r1"`)
	if err != nil || !res.NotModified || res.ETag != `W/"r1"` {
		t.Fatalf("res=%+v err=%v; want NotModified with the old ETag kept", res, err)
	}
}

func TestReleases_rateLimit(t *testing.T) {
	reset := time.Now().Add(20 * time.Minute).Unix()
	cases := []struct {
		name   string
		status int
		hdr    map[string]string
		want   time.Duration // approx. from now; 0 = expect a StatusError
	}{
		{"403 remaining 0", 403, map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": strconv.FormatInt(reset, 10)}, 20 * time.Minute},
		{"429 retry-after", 429, map[string]string{"Retry-After": "120"}, 2 * time.Minute},
		{"429 bare", 429, nil, time.Hour},
		{"403 without headers", 403, nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for k, v := range c.hdr {
					w.Header().Set(k, v)
				}
				w.WriteHeader(c.status)
			}))
			defer srv.Close()
			_, err := testSource(srv.URL).Releases(context.Background(), "")
			var rl *RateLimitError
			var se *netstate.StatusError
			if c.want == 0 {
				if !errors.As(err, &se) || se.Code != c.status {
					t.Fatalf("err = %v, want StatusError %d", err, c.status)
				}
				return
			}
			if !errors.As(err, &rl) {
				t.Fatalf("err = %v, want RateLimitError", err)
			}
			if d := time.Until(rl.Until); d < c.want-time.Minute || d > c.want+time.Minute {
				t.Fatalf("Until in %v, want ≈%v", d, c.want)
			}
		})
	}
}

func TestReleases_serverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	_, err := testSource(srv.URL).Releases(context.Background(), "")
	var se *netstate.StatusError
	if !errors.As(err, &se) || se.Code != 502 {
		t.Fatalf("err = %v, want StatusError 502", err)
	}
}

func TestPakJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"p1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"p1"`)
		w.Write(fixture(t, "pak.json"))
	}))
	defer srv.Close()
	s := testSource(srv.URL)
	rel, etag, nm, err := s.PakJSON(context.Background(), "")
	if err != nil || nm || rel.Tag != "v1.0.25" || etag != `"p1"` ||
		rel.URL != "https://github.com/carroarmato0/NextUI-Itchio-Pak/releases/tag/v1.0.25" {
		t.Fatalf("rel=%+v etag=%q nm=%v err=%v", rel, etag, nm, err)
	}
	if _, _, nm, err := s.PakJSON(context.Background(), `"p1"`); !nm || err != nil {
		t.Fatalf("second fetch: nm=%v err=%v, want not modified", nm, err)
	}
}

func TestPakJSON_badVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"version":"latest"}`))
	}))
	defer srv.Close()
	if _, _, _, err := testSource(srv.URL).PakJSON(context.Background(), ""); err == nil {
		t.Fatal("want an error for a version that is not a release tag")
	}
}

// A truncated or malformed body is GitHub's problem, not the network's: it
// must never classify as offline, or the check is owed instead of failed.
func TestPakJSON_truncatedIsNotOffline(t *testing.T) {
	for _, body := range []string{`{"version":`, `{"version":"v1.0.2`, `not json`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(body))
		}))
		_, _, _, err := testSource(srv.URL).PakJSON(context.Background(), "")
		srv.Close()
		if err == nil {
			t.Fatalf("%q: want an error", body)
		}
		if r := netstate.Classify(err); r.Offline() {
			t.Fatalf("%q: err %v classified %s, want not offline", body, err, r)
		}
	}
}

func TestReleases_malformedIsNotOffline(t *testing.T) {
	for _, body := range []string{`{}`, `[{"tag_name":"v1.1.0-rc3","prerelease":`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(body))
		}))
		_, err := testSource(srv.URL).Releases(context.Background(), "")
		srv.Close()
		if err == nil {
			t.Fatalf("%q: want an error", body)
		}
		if r := netstate.Classify(err); r.Offline() {
			t.Fatalf("%q: err %v classified %s, want not offline", body, err, r)
		}
	}
}

func TestToRelease_fillsNextUIAsset(t *testing.T) {
	r := toRelease(ghRelease{Tag: "v1.1.0-rc5", Assets: []ghAsset{
		{Name: "Itch-io.NextUI.v1.1.0-rc5.pak.zip", URL: "https://x/pak.zip", Size: 10, Digest: "sha256:aa"},
		{Name: "Itch-io.NextUI.v1.1.0-rc5.pakz", URL: "https://x/pakz", Size: 50, Digest: "sha256:bb"},
		{Name: "Itch-io.muOS.v1.1.0-rc5.muxapp", URL: "https://x/mux", Size: 9, Digest: "sha256:cc"},
	}})
	if r.NextUIAsset != "https://x/pak.zip" || r.NextUISize != 10 || r.NextUIDigest != "sha256:aa" {
		t.Fatalf("NextUI asset = %q %d %q", r.NextUIAsset, r.NextUISize, r.NextUIDigest)
	}
	if r.Asset != "https://x/mux" {
		t.Fatalf("muOS asset = %q", r.Asset)
	}
}
