package itchio_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

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

func TestFetchGamesFromURL_403ReturnsCloudflareError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	_, err := c.FetchGamesFromURL(srv.URL + "/games/made-with-gb-studio.xml?page=1")
	if !errors.Is(err, itchio.ErrCloudflareBlocked) {
		t.Fatalf("expected ErrCloudflareBlocked, got %v", err)
	}
}

func TestFetchGamesFromURL_PublishedAt(t *testing.T) {
	xml := `<?xml version="1.0"?>
<rss version="2.0"><channel>
<item>
  <title>Dated Game</title>
  <link>https://dev.itch.io/dated-game</link>
  <description></description>
  <price>0.0</price>
  <pubDate>Fri, 11 Dec 2020 02:30:01 GMT</pubDate>
</item>
<item>
  <title>Undated Game</title>
  <link>https://dev.itch.io/undated-game</link>
  <description></description>
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
	if len(games) != 2 {
		t.Fatalf("want 2 games, got %d", len(games))
	}

	want := time.Date(2020, 12, 11, 2, 30, 1, 0, time.UTC)
	if !games[0].PublishedAt.Equal(want) {
		t.Errorf("PublishedAt = %v, want %v", games[0].PublishedAt, want)
	}
	if !games[1].PublishedAt.IsZero() {
		t.Errorf("undated game PublishedAt should be zero, got %v", games[1].PublishedAt)
	}
}

func TestFetchGamesFromURL_sendsBrowserHeaders(t *testing.T) {
	var gotUA, gotAccept, gotLang, gotFetchMode string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		gotLang = r.Header.Get("Accept-Language")
		gotFetchMode = r.Header.Get("Sec-Fetch-Mode")
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel></channel></rss>`))
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	c.FetchGamesFromURL(srv.URL + "/games/made-with-gb-studio.xml?page=1")

	if gotUA == "" {
		t.Error("User-Agent header not sent")
	}
	if gotAccept == "" {
		t.Error("Accept header not sent")
	}
	if gotLang == "" {
		t.Error("Accept-Language header not sent")
	}
	if gotFetchMode == "" {
		t.Error("Sec-Fetch-Mode header not sent")
	}
}
