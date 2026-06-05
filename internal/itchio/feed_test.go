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

func TestFetchGamesTrailingCurrencySymbol(t *testing.T) {
	cases := []struct {
		price string
		want  bool
	}{
		{"3.39€", false}, // European format: symbol after number
		{"3.99£", false},
		{"$3.99", false}, // Leading symbol still works
		{"0.0", true},
	}
	for _, tc := range cases {
		xml := `<?xml version="1.0"?>
<rss version="2.0"><channel>
<item>
  <title>Test Game</title>
  <link>https://testdev.itch.io/test-game</link>
  <description>&lt;img src="https://img.itch.zone/test.png"/&gt;</description>
  <price>` + tc.price + `</price>
</item>
</channel></rss>`

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(xml))
		}))
		c := itchio.NewClient()
		games, err := c.FetchGamesFromURL(srv.URL)
		srv.Close()
		if err != nil {
			t.Fatalf("price %q: FetchGamesFromURL: %v", tc.price, err)
		}
		if len(games) != 1 {
			t.Fatalf("price %q: want 1 game, got %d", tc.price, len(games))
		}
		if games[0].IsFree != tc.want {
			t.Errorf("price %q: IsFree = %v, want %v", tc.price, games[0].IsFree, tc.want)
		}
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

	// This NES game has a unique URL so it survives deduplication.
	nesPage1XML := `<?xml version="1.0"?><rss version="2.0"><channel>
<item>
  <title>A NES Game</title>
  <link>https://nesdev.itch.io/nes-game</link>
  <description></description>
  <price>0.0</price>
</item>
</channel></rss>`

	emptyFeed := `<?xml version="1.0"?><rss version="2.0"><channel></channel></rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		slug := r.URL.Path   // e.g. "/games/made-with-gb-studio.xml"
		page := r.URL.Query().Get("page")
		switch {
		case slug == "/games/made-with-gb-studio.xml" && page == "1":
			w.Write(page1)
		case slug == "/games/made-with-gb-studio.xml" && page == "2":
			w.Write([]byte(page2XML))
		case slug == "/games/tag-nes-rom.xml" && page == "1":
			w.Write([]byte(nesPage1XML))
		default:
			w.Write([]byte(emptyFeed))
		}
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	var progressCalls int
	var lastFetched int
	games, err := c.FetchAllGames(context.Background(), func(partial []itchio.Game) {
		progressCalls++
		lastFetched = len(partial)
	})
	if err != nil {
		t.Fatalf("FetchAllGames: %v", err)
	}
	// rss_page1.xml has 36 items; page2 has 2 GB games; 1 NES game → total 39.
	if len(games) != 39 {
		t.Errorf("got %d games, want 39", len(games))
	}
	// Progress fires once per page that adds new games: GB page1 (36), GB page2 (2), NES page1 (1).
	if progressCalls != 3 {
		t.Errorf("progress calls = %d, want 3", progressCalls)
	}
	if lastFetched != 39 {
		t.Errorf("last fetched = %d, want 39", lastFetched)
	}
}

func TestFetchAllGames_StopsOnWrapAround(t *testing.T) {
	// itch.io recycles the first page past the last real page instead of
	// returning an empty feed. A full page of all-duplicates must terminate
	// the loop for that slug.
	page1, err := os.ReadFile("../../testdata/rss_page1.xml")
	if err != nil {
		t.Fatalf("read rss_page1.xml: %v", err)
	}

	var pageRequests int
	emptyFeed := `<?xml version="1.0"?><rss version="2.0"><channel></channel></rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		if r.URL.Path == "/games/made-with-gb-studio.xml" {
			pageRequests++
			// Every page returns the same 36 items — simulates itch.io wrap-around.
			w.Write(page1)
		} else {
			w.Write([]byte(emptyFeed))
		}
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	games, err := c.FetchAllGames(context.Background(), nil)
	if err != nil {
		t.Fatalf("FetchAllGames: %v", err)
	}
	// Page 1 adds 36 games; page 2 is all-duplicates → loop must stop after 2 requests.
	if pageRequests != 2 {
		t.Errorf("made %d page requests, want 2 (page 1 adds games, page 2 triggers wrap-around stop)", pageRequests)
	}
	if len(games) != 36 {
		t.Errorf("got %d games, want 36", len(games))
	}
}

func TestFetchAllGames_Dedup(t *testing.T) {
	// The same game URL appears in two different feed slugs.
	// It should only appear once in the result, tagged with the first platform.
	dupXML := `<?xml version="1.0"?><rss version="2.0"><channel>
<item>
  <title>Dup Game</title>
  <link>https://dupdev.itch.io/dup-game</link>
  <description></description>
  <price>0.0</price>
</item>
</channel></rss>`

	emptyFeed := `<?xml version="1.0"?><rss version="2.0"><channel></channel></rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		slug := r.URL.Path
		// Return the duplicate game from both a GBC feed and the GB Studio feed.
		if (slug == "/games/tag-gameboy-color.xml" || slug == "/games/made-with-gb-studio.xml") && r.URL.Query().Get("page") == "1" {
			w.Write([]byte(dupXML))
		} else {
			w.Write([]byte(emptyFeed))
		}
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	games, err := c.FetchAllGames(context.Background(), nil)
	if err != nil {
		t.Fatalf("FetchAllGames: %v", err)
	}
	if len(games) != 1 {
		t.Errorf("got %d games after dedup, want 1", len(games))
	}
	// GBC feeds are processed before GB feeds, so the game should be tagged GBC.
	if games[0].Platform != "GBC" {
		t.Errorf("Platform = %q, want %q", games[0].Platform, "GBC")
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
	var firstServed bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		// Cancel context on the very first request (any feed, page 1).
		// Subsequent requests should not be made.
		if !firstServed {
			firstServed = true
			cancel()
			w.Write(page1)
		} else {
			// After cancellation the context check should prevent further requests.
			t.Errorf("unexpected request after cancellation: %s", r.URL.String())
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := itchio.NewClientWithBase(srv.URL)
	games, err := c.FetchAllGames(ctx, nil)
	if err == nil {
		t.Error("expected error from mid-fetch cancellation, got nil")
	}
	// Partial results (first page served) should still be returned.
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
