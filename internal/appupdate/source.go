package appupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

const (
	repo = "carroarmato0/NextUI-Itchio-Pak"
	// per_page=30: a run of release candidates must not push the newest
	// stable release off the page (spec §1).
	defaultReleasesURL = "https://api.github.com/repos/" + repo + "/releases?per_page=30"
	// The Pak Store's own source for the latest version.
	defaultPakJSONURL = "https://raw.githubusercontent.com/" + repo + "/refs/heads/main/pak.json"
	releasePage       = "https://github.com/" + repo + "/releases/tag/"
	maxBody           = 4 << 20
)

// AssetName is the muOS asset of a release.
func AssetName(tag string) string { return "Itch-io.muOS." + tag + ".muxapp" }

// Source fetches release information from GitHub.
type Source struct {
	HTTP        *http.Client
	UserAgent   string
	ReleasesURL string
	PakJSONURL  string
	Now         func() time.Time
}

// NewSource returns a Source for the real endpoints. userAgent must be the
// short form (itchio.BuildUserAgent(info, false)): device details are for
// itch.io and are not sent to GitHub.
func NewSource(userAgent string) *Source {
	return &Source{
		HTTP:        newClient(10 * time.Second),
		UserAgent:   userAgent,
		ReleasesURL: defaultReleasesURL,
		PakJSONURL:  defaultPakJSONURL,
		Now:         time.Now,
	}
}

// newClient verifies TLS normally (SSL_CERT_FILE points Go at the bundled CA
// file on devices). timeout 0 means none, for the ARCHIVE download, which has
// its own idle timeout.
//
// Deliberately not wrapped in netstate.Transport: the monitor probes itch.io,
// so a GitHub-only failure fed into netstate could never settle (offline,
// probe says online, owed check reruns, fails, offline again...).
func newClient(timeout time.Duration) *http.Client {
	base := http.DefaultTransport.(*http.Transport).Clone()
	return &http.Client{Timeout: timeout, Transport: base}
}

// RateLimitError is GitHub asking us to wait until Until.
type RateLimitError struct{ Until time.Time }

func (e *RateLimitError) Error() string {
	return "GitHub rate limit until " + e.Until.Format(time.RFC3339)
}

// ReleasesResult is the newest release per channel in one releases response.
// One response lists both kinds, so both channels are filled every time.
type ReleasesResult struct {
	Stable, RC  *Release // nil when the page has none of that kind
	ETag        string
	NotModified bool
}

type ghAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

type ghRelease struct {
	Tag        string    `json:"tag_name"`
	HTMLURL    string    `json:"html_url"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	Assets     []ghAsset `json:"assets"`
}

func (s *Source) get(ctx context.Context, url, etag, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.UserAgent)
	req.Header.Set("Accept", accept)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	return s.HTTP.Do(req)
}

// checkStatus turns an unwanted response into an error; 200 and 304 pass.
func (s *Source) checkStatus(resp *http.Response, what string) error {
	switch c := resp.StatusCode; c {
	case http.StatusOK, http.StatusNotModified:
		return nil
	case http.StatusForbidden, http.StatusTooManyRequests:
		if until, ok := rateLimitUntil(resp, s.Now()); ok {
			return &RateLimitError{Until: until}
		}
	}
	return &netstate.StatusError{What: what, Code: resp.StatusCode}
}

func rateLimitUntil(resp *http.Response, now time.Time) (time.Time, bool) {
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil && secs >= 0 {
			return now.Add(time.Duration(secs) * time.Second), true
		}
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			return time.Unix(reset, 0), true
		}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return now.Add(time.Hour), true
	}
	return time.Time{}, false // a 403 that is not a rate limit
}

// Releases fetches the releases list.
func (s *Source) Releases(ctx context.Context, etag string) (ReleasesResult, error) {
	start := s.Now()
	resp, err := s.get(ctx, s.ReleasesURL, etag, "application/vnd.github+json")
	if err != nil {
		return ReleasesResult{}, err
	}
	defer resp.Body.Close()
	if err := s.checkStatus(resp, "GitHub releases"); err != nil {
		return ReleasesResult{}, err
	}
	if resp.StatusCode == http.StatusNotModified {
		logger.Info("appupdate: releases HTTP 304 (ETag hit) in %v", s.Now().Sub(start).Round(time.Millisecond))
		return ReleasesResult{ETag: etag, NotModified: true}, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return ReleasesResult{}, err
	}
	var list []ghRelease
	if err := json.Unmarshal(body, &list); err != nil {
		return ReleasesResult{}, fmt.Errorf("decode releases: %w", err)
	}
	res := pickReleases(list)
	res.ETag = resp.Header.Get("ETag")
	logger.Info("appupdate: releases HTTP 200, %d bytes, %d releases, stable=%s rc=%s in %v",
		len(body), len(list), tagOf(res.Stable), tagOf(res.RC), s.Now().Sub(start).Round(time.Millisecond))
	return res, nil
}

func pickReleases(list []ghRelease) ReleasesResult {
	var res ReleasesResult
	var bestStable, bestRC Version
	for _, r := range list {
		if r.Draft {
			continue
		}
		v, ok := Parse(r.Tag)
		if !ok {
			logger.Debug("appupdate: skipping release %q: not a version tag", r.Tag)
			continue
		}
		rel := toRelease(r)
		if res.RC == nil || Compare(v, bestRC) > 0 {
			res.RC, bestRC = rel, v
		}
		if !r.Prerelease && (res.Stable == nil || Compare(v, bestStable) > 0) {
			res.Stable, bestStable = rel, v
		}
	}
	return res
}

func toRelease(r ghRelease) *Release {
	rel := &Release{Tag: r.Tag, URL: r.HTMLURL}
	if rel.URL == "" {
		rel.URL = releasePage + r.Tag
	}
	want := AssetName(r.Tag)
	for _, a := range r.Assets {
		if a.Name == want {
			rel.Asset, rel.Digest, rel.Size = a.URL, a.Digest, a.Size
		}
	}
	return rel
}

func tagOf(r *Release) string {
	if r == nil {
		return "none"
	}
	return r.Tag
}

// PakJSON fetches pak.json on main: what the Pak Store will offer.
func (s *Source) PakJSON(ctx context.Context, etag string) (*Release, string, bool, error) {
	start := s.Now()
	resp, err := s.get(ctx, s.PakJSONURL, etag, "application/json")
	if err != nil {
		return nil, "", false, err
	}
	defer resp.Body.Close()
	if err := s.checkStatus(resp, "pak.json"); err != nil {
		return nil, "", false, err
	}
	if resp.StatusCode == http.StatusNotModified {
		logger.Info("appupdate: pak.json HTTP 304 (ETag hit) in %v", s.Now().Sub(start).Round(time.Millisecond))
		return nil, etag, true, nil
	}
	// Read first, decode second: a connection dropping mid-body stays a
	// network error, while a truncated or malformed document is a JSON error
	// (json.Unmarshal never reports io.ErrUnexpectedEOF), which must not
	// classify as offline.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, "", false, err
	}
	var pj struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &pj); err != nil {
		logger.Warn("appupdate: pak.json HTTP 200 but %d bytes do not decode: %v", len(body), err)
		return nil, "", false, fmt.Errorf("decode pak.json: %w", err)
	}
	if _, ok := Parse(pj.Version); !ok {
		return nil, "", false, fmt.Errorf("pak.json version %q is not a release tag", pj.Version)
	}
	logger.Info("appupdate: pak.json HTTP 200, version=%s in %v", pj.Version, s.Now().Sub(start).Round(time.Millisecond))
	return &Release{Tag: pj.Version, URL: releasePage + pj.Version}, resp.Header.Get("ETag"), false, nil
}
