package inventory_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

// apiGame is a fake itch.io serving one game through both the page and the
// API. Fields can be changed between update runs.
type apiGame struct {
	t        *testing.T
	mu       sync.Mutex
	price    string // "" free, else paid
	owned    bool   // owned-keys returns a key for the game
	reject   bool   // the API refuses the token
	uploads  []string
	md5      map[string]string
	pageHits int
	apiHits  int
	keyUsed  string
}

func (g *apiGame) server() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/game/download_url", func(w http.ResponseWriter, r *http.Request) {
		g.t.Errorf("update check POSTed to download_url")
	})
	mux.HandleFunc("/game/data.json", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.price == "" {
			w.Write([]byte(`{"id":42}`))
		} else {
			fmt.Fprintf(w, `{"id":42,"price":%q}`, g.price)
		}
	})
	mux.HandleFunc("/game", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.pageHits++
		for _, u := range g.uploads {
			fmt.Fprintf(w, `<div class="upload"><strong class="name" title="%s">%s</strong></div>`, u, u)
		}
	})
	mux.HandleFunc("/profile/owned-keys", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		if r.URL.Query().Get("game_ids") != "42" {
			g.t.Errorf("owned-keys game_ids = %q", r.URL.Query().Get("game_ids"))
		}
		if g.owned {
			w.Write([]byte(`{"per_page":50,"owned_keys":[{"id":77,"game_id":42,"purchase_id":9}]}`))
		} else {
			w.Write([]byte(`{"per_page":50,"owned_keys":{}}`))
		}
	})
	mux.HandleFunc("/games/42/uploads", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.apiHits++
		if g.reject || r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		g.keyUsed = r.URL.Query().Get("download_key_id")
		var parts []string
		for i, u := range g.uploads {
			parts = append(parts, fmt.Sprintf(`{"id":%d,"filename":%q,"md5_hash":%q,"type":"default","traits":{}}`, 100+i, u, g.md5[u]))
		}
		fmt.Fprintf(w, `{"uploads":[%s]}`, strings.Join(parts, ","))
	})
	return httptest.NewServer(mux)
}

// runUpdate installs game.gb for the fake game (on the first call) and runs
// one update check, signed in with "tok" when signedIn.
func runUpdate(t *testing.T, srv *httptest.Server, inv *inventory.Inventory, dir string, free, signedIn bool) string {
	t.Helper()
	gameURL := srv.URL + "/game"
	if _, ok := inv.Lookup(gameURL); !ok {
		romPath := filepath.Join(dir, "game.gb")
		os.WriteFile(romPath, []byte("ROM"), 0644)
		art := inventory.CoverArtPath(srv.URL+"/cover.png", romPath)
		os.MkdirAll(filepath.Dir(art), 0755)
		os.WriteFile(art, minimalPNG(), 0644)
		inv.Add(gameURL, inventory.Entry{Title: "G", IsFree: free, CoverURL: srv.URL + "/cover.png"},
			inventory.DownloadedFile{Filename: "game.gb", DestPath: romPath, DownloadedAt: time.Now()})
	}
	client := itchio.NewClientWithBaseAndButler(srv.URL, srv.URL)
	if signedIn {
		client.SetAuthToken("tok")
	}
	svc := inventory.NewUpdateService(inv, filepath.Join(dir, "inventory.json"), client, nil)
	done := make(chan struct{})
	svc.Start(func() { close(done) })
	<-done
	svc.Stop()
	return gameURL
}

func newInv() *inventory.Inventory {
	return &inventory.Inventory{Entries: make(map[string]*inventory.Entry)}
}

// Signed in, a free game's files come from the API, and a re-upload under
// the same name — undetectable from the page — is an update.
func TestUpdate_signedInFreeUsesAPIAndSeesContentChanges(t *testing.T) {
	g := &apiGame{t: t, uploads: []string{"game.gb"}, md5: map[string]string{"game.gb": "aaa"}}
	srv := g.server()
	defer srv.Close()
	inv, dir := newInv(), t.TempDir()

	url := runUpdate(t, srv, inv, dir, true, true)
	if inv.HasPendingUpdates(url) {
		t.Fatal("pending after the first check")
	}
	g.mu.Lock()
	g.md5["game.gb"] = "bbb"
	g.mu.Unlock()
	runUpdate(t, srv, inv, dir, true, true)
	if !inv.HasPendingUpdates(url) {
		t.Error("new content under the same name not reported")
	}
	if g.pageHits != 0 || g.apiHits != 2 {
		t.Errorf("page fetched %d times, API %d; signed in should use only the API", g.pageHits, g.apiHits)
	}
	if g.keyUsed != "" {
		t.Errorf("free game listed with download_key_id=%q", g.keyUsed)
	}
}

// A paid game is listed with its download key, from one owned-keys request.
func TestUpdate_signedInPaidUsesOwnedKey(t *testing.T) {
	g := &apiGame{t: t, price: "$5.00", owned: true, uploads: []string{"game.gb"}, md5: map[string]string{}}
	srv := g.server()
	defer srv.Close()
	inv, dir := newInv(), t.TempDir()

	url := runUpdate(t, srv, inv, dir, false, true)
	g.mu.Lock()
	g.uploads = append(g.uploads, "game-v2.gb")
	g.mu.Unlock()
	runUpdate(t, srv, inv, dir, false, true)
	if g.keyUsed != "77" {
		t.Errorf("uploads listed with download_key_id=%q, want 77", g.keyUsed)
	}
	if !inv.HasPendingUpdates(url) {
		t.Error("new upload of a paid game not reported")
	}
}

// Signed in but not owning a paid game (a refund), the page is used.
func TestUpdate_signedInPaidNotOwnedUsesThePage(t *testing.T) {
	g := &apiGame{t: t, price: "$5.00", uploads: []string{"game.gb"}, md5: map[string]string{}}
	srv := g.server()
	defer srv.Close()
	runUpdate(t, srv, newInv(), t.TempDir(), false, true)
	if g.pageHits != 1 || g.apiHits != 0 {
		t.Errorf("page %d, API %d; want the page only", g.pageHits, g.apiHits)
	}
}

// A token the API refuses falls back to the page instead of failing.
func TestUpdate_rejectedTokenFallsBackToThePage(t *testing.T) {
	g := &apiGame{t: t, reject: true, uploads: []string{"game.gb"}, md5: map[string]string{}}
	srv := g.server()
	defer srv.Close()
	inv := newInv()
	url := runUpdate(t, srv, inv, t.TempDir(), true, true)
	e, _ := inv.Lookup(url)
	if g.pageHits != 1 || len(e.KnownUpstreamFiles) != 1 {
		t.Errorf("page hits %d, upstream %v", g.pageHits, e.KnownUpstreamFiles)
	}
}

// Signed out, a paid game's page is read too: an update is seen, and only
// downloading it needs a sign-in.
func TestUpdate_signedOutPaidReadsThePage(t *testing.T) {
	g := &apiGame{t: t, price: "$5.00", uploads: []string{"game.gb"}, md5: map[string]string{}}
	srv := g.server()
	defer srv.Close()
	inv, dir := newInv(), t.TempDir()
	url := runUpdate(t, srv, inv, dir, false, false)
	g.mu.Lock()
	g.uploads = append(g.uploads, "game-v2.gb")
	g.mu.Unlock()
	runUpdate(t, srv, inv, dir, false, false)
	if !inv.HasPendingUpdates(url) {
		t.Error("signed out: new upload of an installed paid game not reported")
	}
	if g.apiHits != 0 {
		t.Errorf("signed out, but the API was called %d times", g.apiHits)
	}
}
