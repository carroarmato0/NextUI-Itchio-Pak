package itchio_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

func serveFile(t *testing.T, path string) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
}

func TestFetchGameDetailExtractsGameID(t *testing.T) {
	srv := serveFile(t, "../../testdata/game_page_free.html")
	defer srv.Close()

	c := itchio.NewClient()
	detail, err := c.FetchGameDetail(srv.URL)
	if err != nil {
		t.Fatalf("FetchGameDetail: %v", err)
	}
	if detail.GameID == "" {
		t.Error("GameID is empty — game id not found in page")
	}
}

func TestFetchGameDetailExtractsPageTags(t *testing.T) {
	srv := serveFile(t, "../../testdata/game_page_free.html")
	defer srv.Close()

	c := itchio.NewClient()
	detail, err := c.FetchGameDetail(srv.URL)
	if err != nil {
		t.Fatalf("FetchGameDetail: %v", err)
	}
	if len(detail.PageTags) == 0 {
		t.Fatal("PageTags is empty — tag links not found in page")
	}
	// The free fixture (Opossum Country) has "horror" slug among its tags
	found := false
	for _, tag := range detail.PageTags {
		if tag == "horror" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected tag 'horror' in PageTags, got: %v", detail.PageTags)
	}
}

func TestFetchGameDetailPaidDoesNotCrash(t *testing.T) {
	srv := serveFile(t, "../../testdata/game_page_paid.html")
	defer srv.Close()

	c := itchio.NewClient()
	_, err := c.FetchGameDetail(srv.URL)
	if err != nil {
		t.Fatalf("FetchGameDetail on paid page: %v", err)
	}
}

func TestFetchGameDetailExtractsScreenshots(t *testing.T) {
	// itch.io sometimes puts src before class, sometimes class before src.
	const pageHTML = `<html><body>
<img src="https://img.itch.zone/a.png" class="screenshot" alt="">
<img class="screenshot" src="https://img.itch.zone/b.png" alt="">
<img src="https://img.itch.zone/cover.png" class="game_thumb" alt="">
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(pageHTML))
	}))
	defer srv.Close()

	c := itchio.NewClient()
	detail, err := c.FetchGameDetail(srv.URL)
	if err != nil {
		t.Fatalf("FetchGameDetail: %v", err)
	}
	if len(detail.ScreenshotURLs) != 2 {
		t.Fatalf("expected 2 screenshots, got %d: %v", len(detail.ScreenshotURLs), detail.ScreenshotURLs)
	}
	if detail.ScreenshotURLs[0] != "https://img.itch.zone/a.png" {
		t.Errorf("ScreenshotURLs[0] = %q", detail.ScreenshotURLs[0])
	}
	if detail.ScreenshotURLs[1] != "https://img.itch.zone/b.png" {
		t.Errorf("ScreenshotURLs[1] = %q", detail.ScreenshotURLs[1])
	}
}

func TestFetchGameDetailExtractsDescription(t *testing.T) {
	// Simple case: description div contains only paragraphs.
	const simpleHTML = `<html><body>
<div class="formatted_description user_formatted">
<p>First paragraph of the description.</p>
<p>Second paragraph.</p>
</div>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(simpleHTML))
	}))
	defer srv.Close()

	c := itchio.NewClient()
	detail, err := c.FetchGameDetail(srv.URL)
	if err != nil {
		t.Fatalf("FetchGameDetail: %v", err)
	}
	if detail.Description == "" {
		t.Fatal("Description is empty")
	}
	if !strings.Contains(detail.Description, "First paragraph") {
		t.Errorf("Description missing expected text, got: %q", detail.Description)
	}
}

func TestFetchGameDetailExtractsDescriptionWithNestedDiv(t *testing.T) {
	// Regression: Tobu Tobu Girl Deluxe has a YouTube embed wrapper div as the
	// first child of formatted_description. The old regex stopped at that inner
	// </div> and returned an empty description. The HTML-parser approach must
	// skip the embed and still return the paragraph text.
	const pageHTML = `<html><body>
<div class="formatted_description user_formatted">
  <div>
    <button type="button" class="embed_preload youtube_preload">YouTube embed</button>
  </div>
  <p>Your cat is floating into the atmosphere and you are the only one who can save it.</p>
  <p>Tobu Tobu Girl is a fun and challenging arcade platformer.</p>
</div>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(pageHTML))
	}))
	defer srv.Close()

	c := itchio.NewClient()
	detail, err := c.FetchGameDetail(srv.URL)
	if err != nil {
		t.Fatalf("FetchGameDetail: %v", err)
	}
	if detail.Description == "" {
		t.Fatal("Description is empty — nested div before text broke extraction")
	}
	if strings.Contains(detail.Description, "YouTube") {
		t.Errorf("Description should not contain embed button text, got: %q", detail.Description)
	}
	if !strings.Contains(detail.Description, "floating into the atmosphere") {
		t.Errorf("Description missing paragraph text, got: %q", detail.Description)
	}
}

func TestFetchGameDetailExtractsBundleNames(t *testing.T) {
	// Alpha appears twice (page renders it in purchase banner AND related-items section).
	// Beta appears once. Result should be: [Alpha Bundle, Beta Bundle] — deduplicated.
	const pageHTML = `<html><body>
<div class="purchase_banner_inner">
  <div class="bundle_title"><a href="https://itch.io/b/1234/alpha-bundle">Alpha Bundle</a></div>
</div>
<div class="purchase_banner_inner">
  <div class="bundle_title"><a href="https://itch.io/b/5678/beta-bundle">Beta Bundle</a></div>
</div>
<div class="related_section">
  <div class="bundle_title"><a href="https://itch.io/b/1234/alpha-bundle">Alpha Bundle</a></div>
</div>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(pageHTML))
	}))
	defer srv.Close()

	c := itchio.NewClient()
	detail, err := c.FetchGameDetail(srv.URL)
	if err != nil {
		t.Fatalf("FetchGameDetail: %v", err)
	}
	if len(detail.BundleNames) != 2 {
		t.Fatalf("expected 2 bundle names, got %d: %v", len(detail.BundleNames), detail.BundleNames)
	}
	if detail.BundleNames[0] != "Alpha Bundle" {
		t.Errorf("BundleNames[0] = %q, want %q", detail.BundleNames[0], "Alpha Bundle")
	}
	if detail.BundleNames[1] != "Beta Bundle" {
		t.Errorf("BundleNames[1] = %q, want %q", detail.BundleNames[1], "Beta Bundle")
	}
}

func TestFetchGameDetailNoBundleNames(t *testing.T) {
	const pageHTML = `<html><body><p>No bundle here.</p></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(pageHTML))
	}))
	defer srv.Close()

	c := itchio.NewClient()
	detail, err := c.FetchGameDetail(srv.URL)
	if err != nil {
		t.Fatalf("FetchGameDetail: %v", err)
	}
	if len(detail.BundleNames) != 0 {
		t.Errorf("expected no bundle names, got %v", detail.BundleNames)
	}
}

func TestAnnotateBundleNames(t *testing.T) {
	keys := []itchio.OwnedKey{
		{ID: 1, BundleSize: 1},                                   // individual
		{ID: 2, BundleSize: 50, CreatedAt: mustParseTime("2026-04-26")}, // bundle B (newer)
		{ID: 3, BundleSize: 30, CreatedAt: mustParseTime("2026-04-25")}, // bundle A (older)
	}
	bundleNames := []string{"Alpha Bundle", "Beta Bundle"}

	got := itchio.AnnotateBundleNames(keys, bundleNames)

	// Individual key must be untouched.
	if got[0].BundleName != "" {
		t.Errorf("individual key BundleName = %q, want empty", got[0].BundleName)
	}
	// Older bundle key (ID=3, Apr 25) → "Alpha Bundle"
	if got[2].BundleName != "Alpha Bundle" {
		t.Errorf("key id=3 BundleName = %q, want %q", got[2].BundleName, "Alpha Bundle")
	}
	// Newer bundle key (ID=2, Apr 26) → "Beta Bundle"
	if got[1].BundleName != "Beta Bundle" {
		t.Errorf("key id=2 BundleName = %q, want %q", got[1].BundleName, "Beta Bundle")
	}
}

func TestAnnotateBundleNames_NoBundles(t *testing.T) {
	keys := []itchio.OwnedKey{{ID: 1, BundleSize: 1}}
	got := itchio.AnnotateBundleNames(keys, []string{"Alpha Bundle"})
	if got[0].BundleName != "" {
		t.Errorf("individual key should not get a bundle name")
	}
}

func TestAnnotateBundleNames_MoreKeysThanNames(t *testing.T) {
	keys := []itchio.OwnedKey{
		{ID: 1, BundleSize: 10, CreatedAt: mustParseTime("2026-01-01")},
		{ID: 2, BundleSize: 20, CreatedAt: mustParseTime("2026-01-02")},
	}
	got := itchio.AnnotateBundleNames(keys, []string{"Only Bundle"})
	if got[0].BundleName != "Only Bundle" {
		t.Errorf("key[0] BundleName = %q, want %q", got[0].BundleName, "Only Bundle")
	}
	if got[1].BundleName != "" {
		t.Errorf("key[1] BundleName = %q, want empty (no name available)", got[1].BundleName)
	}
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestParseDownloadPage_UnknownExt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
<head><meta name="csrf_token" value="CSRF"/></head>
<body>
<div class="upload_list_widget">
  <div class="upload">
    <div class="info_column"><div class="upload_name">
      <strong class="name" title="game.gbc">game.gbc</strong>
    </div></div>
    <div class="actions">
      <a class="button download_btn" href="javascript:void(0);" data-upload_id="1">Download</a>
    </div>
  </div>
  <div class="upload">
    <div class="info_column"><div class="upload_name">
      <strong class="name" title="Glory Hunters 2.0">Glory Hunters 2.0</strong>
    </div></div>
    <div class="actions">
      <a class="button download_btn" href="javascript:void(0);" data-upload_id="2">Download</a>
    </div>
  </div>
  <div class="upload">
    <div class="info_column"><div class="upload_name">
      <strong class="name" title="manual.pdf">manual.pdf</strong>
    </div></div>
    <div class="actions">
      <a class="button download_btn" href="javascript:void(0);" data-upload_id="3">Download</a>
    </div>
  </div>
</div>
</body></html>`))
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	result, err := c.ParseDownloadPage(srv.URL + "/dl/TOKEN")
	if err != nil {
		t.Fatalf("ParseDownloadPage: %v", err)
	}
	// manual.pdf must be dropped; game.gbc and Glory Hunters 2.0 must be kept
	if len(result.Uploads) != 2 {
		t.Fatalf("expected 2 uploads, got %d", len(result.Uploads))
	}

	gbc := result.Uploads[0]
	if gbc.Filename != "game.gbc" {
		t.Errorf("uploads[0].Filename = %q, want game.gbc", gbc.Filename)
	}
	if gbc.NeedsFormat {
		t.Errorf("uploads[0].NeedsFormat = true, want false for .gbc file")
	}

	unknown := result.Uploads[1]
	if unknown.Filename != "Glory Hunters 2.0" {
		t.Errorf("uploads[1].Filename = %q, want 'Glory Hunters 2.0'", unknown.Filename)
	}
	if !unknown.NeedsFormat {
		t.Errorf("uploads[1].NeedsFormat = false, want true for unknown ext")
	}

	for _, u := range result.Uploads {
		if u.Filename == "manual.pdf" {
			t.Errorf("manual.pdf should have been dropped (skippable ext)")
		}
	}
}

func TestParseDownloadPage_ZipTreatedAsKnown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
<head><meta name="csrf_token" value="CSRF"/></head>
<body>
<div class="upload_list_widget">
  <div class="upload">
    <div class="info_column"><div class="upload_name">
      <strong class="name" title="game.gbc">game.gbc</strong>
    </div></div>
    <div class="actions">
      <a class="button download_btn" href="javascript:void(0);" data-upload_id="1">Download</a>
    </div>
  </div>
  <div class="upload">
    <div class="info_column"><div class="upload_name">
      <strong class="name" title="Glory Hunters 2.0.zip">Glory Hunters 2.0.zip</strong>
    </div></div>
    <div class="actions">
      <a class="button download_btn" href="javascript:void(0);" data-upload_id="2">Download</a>
    </div>
  </div>
  <div class="upload">
    <div class="info_column"><div class="upload_name">
      <strong class="name" title="manual.pdf">manual.pdf</strong>
    </div></div>
    <div class="actions">
      <a class="button download_btn" href="javascript:void(0);" data-upload_id="3">Download</a>
    </div>
  </div>
</div>
</body></html>`))
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	result, err := c.ParseDownloadPage(srv.URL + "/dl/TOKEN")
	if err != nil {
		t.Fatalf("ParseDownloadPage: %v", err)
	}
	// manual.pdf dropped; game.gbc and Glory Hunters 2.0.zip kept, both NeedsFormat=false
	if len(result.Uploads) != 2 {
		t.Fatalf("expected 2 uploads, got %d", len(result.Uploads))
	}
	for _, u := range result.Uploads {
		if u.NeedsFormat {
			t.Errorf("%q has NeedsFormat=true, want false (.zip should be treated as known)", u.Filename)
		}
		if u.Filename == "manual.pdf" {
			t.Errorf("manual.pdf should have been dropped")
		}
	}
}

func TestParseDownloadPageFiltersROMs(t *testing.T) {
	srv := serveFile(t, "../../testdata/download_page.html")
	defer srv.Close()

	c := itchio.NewClient()
	result, err := c.ParseDownloadPage(srv.URL)
	if err != nil {
		t.Fatalf("ParseDownloadPage: %v", err)
	}
	if len(result.Uploads) == 0 {
		t.Fatal("expected at least one upload")
	}
	for _, u := range result.Uploads {
		ext := strings.ToLower(filepath.Ext(u.Filename))
		if ext != ".gbc" && ext != ".gb" {
			t.Errorf("unexpected non-ROM upload: %q", u.Filename)
		}
	}
	// Should have found both .gbc and .gb from our fixture
	if len(result.Uploads) != 2 {
		t.Errorf("expected 2 ROM uploads, got %d", len(result.Uploads))
	}
}
