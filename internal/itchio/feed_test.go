package itchio_test

import (
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
