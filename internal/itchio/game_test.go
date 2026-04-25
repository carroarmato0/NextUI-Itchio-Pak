package itchio_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
