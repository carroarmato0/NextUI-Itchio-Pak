package itchio_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

// serveData serves testdata/data_<kind>.json at /<game>/data.json.
func serveData(t *testing.T, kind string) *httptest.Server {
	t.Helper()
	body, err := os.ReadFile("../../testdata/data_" + kind + ".json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/data.json") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Fixtures are real itch.io responses (2026-09-25): Glory Hunters (paid, on
// sale), Opossum Country (name-your-own-price), CrissCross Cove (free).
func TestFetchGameData_pricing(t *testing.T) {
	for _, tc := range []struct {
		kind        string
		id          int64
		pricing     itchio.PricingModel
		price       string
		suggested   string
		original    string
		onSale      bool
		screenshots int
		firstTag    string
	}{
		{"paid", 1206111, itchio.PricingPaid, "$2.75", "$3.85", "$5.00", true, 4, "8-bit"},
		{"nyop", 850892, itchio.PricingNameYourOwnPrice, "$0.00", "$2.00", "", false, 4, ""},
		{"free", 5033642, itchio.PricingFree, "", "", "", false, 3, ""},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			srv := serveData(t, tc.kind)
			c := itchio.NewClientWithBase(srv.URL)
			d, err := c.FetchGameData(srv.URL + "/game")
			if err != nil {
				t.Fatalf("FetchGameData: %v", err)
			}
			if d.ID != tc.id || d.Pricing() != tc.pricing || d.Price != tc.price ||
				d.SuggestedPrice != tc.suggested || d.OriginalPrice != tc.original || (d.Sale != nil) != tc.onSale {
				t.Errorf("got %+v, pricing %v", d, d.Pricing())
			}
			if len(d.Screenshots) != tc.screenshots || (tc.firstTag != "" && d.Tags[0] != tc.firstTag) {
				t.Errorf("screenshots %d, tags %v", len(d.Screenshots), d.Tags)
			}
			if d.CoverImage == "" || len(d.Authors) == 0 || d.Authors[0].Name == "" {
				t.Errorf("cover or authors missing: %+v", d)
			}
		})
	}
}

func TestFetchGameData_saleDetails(t *testing.T) {
	srv := serveData(t, "paid")
	d, err := itchio.NewClientWithBase(srv.URL).FetchGameData(srv.URL + "/game")
	if err != nil {
		t.Fatal(err)
	}
	if d.Sale == nil || d.Sale.Rate != 45 || d.Sale.EndDate == "" {
		t.Errorf("sale = %+v", d.Sale)
	}
}

func TestFetchGameData_missingGameIsRemoved(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	_, err := itchio.NewClientWithBase(srv.URL).FetchGameData(srv.URL + "/gone")
	if !errors.Is(err, itchio.ErrGameRemoved) {
		t.Errorf("err = %v, want ErrGameRemoved", err)
	}
}

// A renamed game redirects; the data comes from the new address, and its
// own link says where the game lives now.
func TestFetchGameData_followsRename(t *testing.T) {
	body, _ := os.ReadFile("../../testdata/data_paid.json")
	mux := http.NewServeMux()
	mux.HandleFunc("/old-name/data.json", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/glory-hunters/data.json", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/glory-hunters/data.json", func(w http.ResponseWriter, r *http.Request) { w.Write(body) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d, err := itchio.NewClientWithBase(srv.URL).FetchGameData(srv.URL + "/old-name")
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != 1206111 || d.URL != "https://2think.itch.io/glory-hunters" {
		t.Errorf("after the redirect: id %d, url %q", d.ID, d.URL)
	}
}

// FetchGameDetail takes the game ID, tags, screenshots, pricing and the
// suggested price from data.json, and still reads the page for what only
// the page has (description, CSRF token, bundles).
func TestFetchGameDetail_usesDataJSON(t *testing.T) {
	page, _ := os.ReadFile("../../testdata/game_page_nyop.html")
	data, _ := os.ReadFile("../../testdata/data_nyop.json")
	var purchaseHits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/game", func(w http.ResponseWriter, r *http.Request) { w.Write(page) })
	mux.HandleFunc("/game/data.json", func(w http.ResponseWriter, r *http.Request) { w.Write(data) })
	mux.HandleFunc("/game/purchase", func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&purchaseHits, 1) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d, err := itchio.NewClientWithBase(srv.URL).FetchGameDetail(srv.URL + "/game")
	if err != nil {
		t.Fatal(err)
	}
	if d.GameID != "850892" || d.Pricing != itchio.PricingNameYourOwnPrice || d.SuggestedPrice != "$2.00" {
		t.Errorf("GameID %q, pricing %v, suggested %q", d.GameID, d.Pricing, d.SuggestedPrice)
	}
	if len(d.ScreenshotURLs) != 4 || len(d.PageTags) == 0 {
		t.Errorf("screenshots %d, tags %v", len(d.ScreenshotURLs), d.PageTags)
	}
	if d.Description == "" {
		t.Error("description, which only the page has, is empty")
	}
	if purchaseHits != 0 {
		t.Errorf("the /purchase page was fetched %d times; data.json carries the suggested price", purchaseHits)
	}
}

// If data.json cannot be read, the page scrape still gives a usable detail.
func TestFetchGameDetail_fallsBackWithoutDataJSON(t *testing.T) {
	page, _ := os.ReadFile("../../testdata/game_page_paid.html")
	mux := http.NewServeMux()
	mux.HandleFunc("/game", func(w http.ResponseWriter, r *http.Request) { w.Write(page) })
	mux.HandleFunc("/game/data.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d, err := itchio.NewClientWithBase(srv.URL).FetchGameDetail(srv.URL + "/game")
	if err != nil {
		t.Fatal(err)
	}
	if d.GameID == "" || d.Pricing != itchio.PricingPaid {
		t.Errorf("fallback: GameID %q, pricing %v", d.GameID, d.Pricing)
	}
}
