package itchio_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

// Real pages (2026-09-25): the public page lists its files without the
// download_url POST. Attribute order varies (title before class).
//
// Files the download flow skips by extension are left out too.
func TestFetchPageUploadNames_skipsLikeTheDownloadFlow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<div class="upload"><strong class="name" title="game.pocket">x</strong></div>
			<div class="upload"><strong class="name" title="cover.jpg">x</strong></div>
			<div class="upload"><strong title="game.gbc" class="name">game.gbc</strong></div>`))
	}))
	defer srv.Close()
	got, err := itchio.NewClientWithBase(srv.URL).FetchPageUploadNames(srv.URL + "/g")
	if err != nil || !reflect.DeepEqual(got, []string{"game.gbc"}) {
		t.Errorf("got %v, %v", got, err)
	}
}

func TestFetchPageUploadNames_realPages(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		want    []string
	}{
		{"game_page_live_nyop.html", []string{"tobudx.gb"}},
		// The page shows an upload's display name when it has one — the same
		// names the old download-page flow recorded, so the inventory agrees.
		{"game_page_live_free.html", []string{"web_1.3.zip", "Criss Cross Cove (GB Rom)", "Printable Map"}},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			page, err := os.ReadFile("../../testdata/" + tc.fixture)
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("%s %s: only a GET of the page is allowed", r.Method, r.URL.Path)
				}
				w.Write(page)
			}))
			defer srv.Close()
			got, err := itchio.NewClientWithBase(srv.URL).FetchPageUploadNames(srv.URL + "/game")
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tc.want {
				found := false
				for _, g := range got {
					found = found || g == w
				}
				if !found {
					t.Errorf("missing %q in %v", w, got)
				}
			}
		})
	}
}

func TestFetchPageUploadNames_removedGame(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	if _, err := itchio.NewClientWithBase(srv.URL).FetchPageUploadNames(srv.URL + "/gone"); err == nil || !strings.Contains(err.Error(), "removed") {
		t.Errorf("err = %v, want ErrGameRemoved", err)
	}
}

// One owned-keys request for every installed paid game, not one per game.
func TestOwnedKeysForGames_oneRequest(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.URL.Query().Get("game_ids"); got != "1,2,3" {
			t.Errorf("game_ids = %q", got)
		}
		w.Write([]byte(`{"per_page":50,"owned_keys":[
			{"id":11,"game_id":1,"purchase_id":100},
			{"id":12,"game_id":1,"purchase_id":200},
			{"id":33,"game_id":3,"purchase_id":300}]}`))
	}))
	defer srv.Close()
	c := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	got, err := c.OwnedKeysForGames("tok", []string{"1", "2", "3"})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Errorf("%d requests, want 1", requests)
	}
	want := map[string]string{"1": "11", "3": "33"} // the first key of each owned game
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestClientAuthToken(t *testing.T) {
	c := itchio.NewClient()
	if c.AuthToken() != "" {
		t.Error("a new client has a token")
	}
	c.SetAuthToken("t")
	if c.AuthToken() != "t" {
		t.Error("token not stored")
	}
	c.SetAuthToken("")
	if c.AuthToken() != "" {
		t.Error("sign-out did not clear the token")
	}
}
