package itchio_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

func TestFetchGamesFromURL(t *testing.T) {
	data, err := os.ReadFile("../../testdata/rss_page1.xml")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write(data)
	}))
	defer srv.Close()

	c := itchio.NewClient()
	games, err := c.FetchGamesFromURL(srv.URL)
	if err != nil {
		t.Fatalf("FetchGamesFromURL: %v", err)
	}
	if len(games) == 0 {
		t.Fatal("expected at least 1 game, got 0")
	}

	g := games[0]
	if g.Title == "" {
		t.Error("game.Title is empty")
	}
	if g.URL == "" {
		t.Error("game.URL is empty")
	}
	if g.CoverURL == "" {
		t.Error("game.CoverURL is empty")
	}
	if g.Author == "" {
		t.Error("game.Author is empty")
	}
}

func TestFetchGamesFreePriceParsing(t *testing.T) {
	xml := `<?xml version="1.0"?>
<rss version="2.0"><channel>
<item>
  <title>Test Game</title>
  <link>https://testdev.itch.io/test-game</link>
  <description>&lt;img src="https://img.itch.zone/test.png"/&gt;</description>
  <price>0.0</price>
</item>
</channel></rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(xml))
	}))
	defer srv.Close()

	c := itchio.NewClient()
	games, err := c.FetchGamesFromURL(srv.URL)
	if err != nil {
		t.Fatalf("FetchGamesFromURL: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("want 1 game, got %d", len(games))
	}
	if !games[0].IsFree {
		t.Error("game with price 0.0 should be IsFree=true")
	}
	if games[0].Author != "testdev" {
		t.Errorf("Author = %q, want %q", games[0].Author, "testdev")
	}
}

func TestFetchAllGames(t *testing.T) {
	page1, err := os.ReadFile("../../testdata/rss_page1.xml")
	if err != nil {
		t.Fatalf("read rss_page1.xml: %v", err)
	}

	page2XML := `<?xml version="1.0"?><rss version="2.0"><channel>
<item>
  <title>Extra Game One</title>
  <link>https://extradev.itch.io/extra-one</link>
  <description></description>
  <price>0.0</price>
</item>
<item>
  <title>Extra Game Two</title>
  <link>https://extradev.itch.io/extra-two</link>
  <description></description>
  <price>0.0</price>
</item>
</channel></rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		switch page {
		case "1":
			w.Header().Set("Content-Type", "application/rss+xml")
			w.Write(page1)
		case "2":
			w.Header().Set("Content-Type", "application/rss+xml")
			w.Write([]byte(page2XML))
		default:
			// Page 3+ returns empty feed → signals end of results.
			w.Header().Set("Content-Type", "application/rss+xml")
			w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel></channel></rss>`))
		}
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	var progressCalls int
	var lastFetched int
	games, err := c.FetchAllGames(context.Background(), func(fetched int) {
		progressCalls++
		lastFetched = fetched
	})
	if err != nil {
		t.Fatalf("FetchAllGames: %v", err)
	}
	// rss_page1.xml has 36 items; page2 has 2 → total 38.
	if len(games) != 38 {
		t.Errorf("got %d games, want 38", len(games))
	}
	if progressCalls != 2 {
		t.Errorf("progress calls = %d, want 2", progressCalls)
	}
	if lastFetched != 38 {
		t.Errorf("last fetched = %d, want 38", lastFetched)
	}
}

func TestFetchAllGames_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel></channel></rss>`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	c := itchio.NewClientWithBase(srv.URL)
	_, err := c.FetchAllGames(ctx, nil)
	if err == nil {
		t.Error("expected error from cancelled context, got nil")
	}
}

func TestFetchAllGames_MidFetchCancellation(t *testing.T) {
	page1, err := os.ReadFile("../../testdata/rss_page1.xml")
	if err != nil {
		t.Fatalf("read rss_page1.xml: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "1" {
			// Cancel context after serving page 1, before page 2 can be requested.
			cancel()
			w.Header().Set("Content-Type", "application/rss+xml")
			w.Write(page1)
		} else {
			// Should not be reached.
			t.Errorf("unexpected request for page %s after cancellation", page)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	games, err := c.FetchAllGames(ctx, nil)
	if err == nil {
		t.Error("expected error from mid-fetch cancellation, got nil")
	}
	// Partial results (page 1) should still be returned.
	if len(games) != 36 {
		t.Errorf("got %d games from partial fetch, want 36", len(games))
	}
}
