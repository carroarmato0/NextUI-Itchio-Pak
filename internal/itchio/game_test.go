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

func TestFetchGameDetailPaidDoesNotCrash(t *testing.T) {
	srv := serveFile(t, "../../testdata/game_page_paid.html")
	defer srv.Close()

	c := itchio.NewClient()
	_, err := c.FetchGameDetail(srv.URL)
	if err != nil {
		t.Fatalf("FetchGameDetail on paid page: %v", err)
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
