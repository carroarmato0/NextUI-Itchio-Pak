# Game List Cache Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cache the complete itch.io GB Studio RSS feed on-device so the game list loads instantly on subsequent launches, while falling back to live page-by-page fetching when no cache exists.

**Architecture:** On first launch (no cache), the existing live page-by-page fetch continues unchanged while a background goroutine silently fetches all pages and writes a `games_cache.json` file. On subsequent launches, the full cached game list is read instantly from disk, and a background goroutine refreshes the cache if it is older than 24 hours. Once the cache is ready, page navigation is served from memory (no network). A "Refresh Game List" item in Settings triggers an immediate full re-fetch.

**Tech Stack:** Go standard library (`encoding/json`, `os`, `context`, `sync`), existing `itchio.Client`, SDL2 event loop pattern already used throughout.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/itchio/cache.go` | **Create** | `GameCache`/`CacheMeta` types, `SaveGamesCache`, `LoadGamesCache` — atomic write via temp-file rename |
| `internal/itchio/cache_test.go` | **Create** | Round-trip save/load, corrupt-file tolerance, missing-file tolerance |
| `internal/itchio/feed.go` | **Modify** | Add `FetchAllGames(ctx, progress)`; fix `FetchGames` to use `c.base` (enables testability) |
| `internal/itchio/feed_test.go` | **Modify** | Add `TestFetchAllGames` using a multi-page httptest server |
| `internal/ui/screen_list.go` | **Modify** | Add `cachedGames`, `cacheReady`, `cachePath` fields; dual-mode `loadPage`; `buildCache`, `refreshCacheIfStale`, `pageSlice`, `triggerCacheRefresh` |
| `internal/ui/screen_settings.go` | **Modify** | Add `sItemRefreshCache` item and `onRefreshGames func()` callback field |
| `cmd/itchio-pak/main_sdl.go` | **Modify** | Derive `cachePath` from `cfgPath`, pass to `NewListScreen`; pass `triggerCacheRefresh` to `NewSettingsScreen` |

---

### Task 1: Cache persistence layer (`internal/itchio/cache.go`)

**Files:**
- Create: `internal/itchio/cache.go`
- Create: `internal/itchio/cache_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/itchio/cache_test.go
package itchio_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

func TestSaveAndLoadGamesCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "games_cache.json")

	games := []itchio.Game{
		{Title: "Alpha", Author: "dev1", URL: "https://dev1.itch.io/alpha", IsFree: true},
		{Title: "Beta", Author: "dev2", URL: "https://dev2.itch.io/beta", Price: 4.99},
	}

	if err := itchio.SaveGamesCache(path, games); err != nil {
		t.Fatalf("SaveGamesCache: %v", err)
	}

	cache, err := itchio.LoadGamesCache(path)
	if err != nil {
		t.Fatalf("LoadGamesCache: %v", err)
	}
	if len(cache.Games) != 2 {
		t.Fatalf("got %d games, want 2", len(cache.Games))
	}
	if cache.Games[0].Title != "Alpha" {
		t.Errorf("Games[0].Title = %q, want %q", cache.Games[0].Title, "Alpha")
	}
	if cache.Games[1].Price != 4.99 {
		t.Errorf("Games[1].Price = %v, want 4.99", cache.Games[1].Price)
	}
	if cache.Meta.TotalGames != 2 {
		t.Errorf("Meta.TotalGames = %d, want 2", cache.Meta.TotalGames)
	}
	if cache.Meta.FetchedAt.IsZero() {
		t.Error("Meta.FetchedAt should not be zero")
	}
	if time.Since(cache.Meta.FetchedAt) > 5*time.Second {
		t.Error("Meta.FetchedAt should be recent")
	}
}

func TestLoadGamesCache_MissingFile(t *testing.T) {
	_, err := itchio.LoadGamesCache("/tmp/does-not-exist-xyz.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadGamesCache_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_cache.json")
	if err := os.WriteFile(path, []byte("not json {{{"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := itchio.LoadGamesCache(path)
	if err == nil {
		t.Fatal("expected error for corrupt file, got nil")
	}
}

func TestSaveGamesCache_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "games_cache.json")

	// Save once, then overwrite — previous data should not leak.
	_ = itchio.SaveGamesCache(path, []itchio.Game{{Title: "Old"}})
	if err := itchio.SaveGamesCache(path, []itchio.Game{{Title: "New"}}); err != nil {
		t.Fatalf("second SaveGamesCache: %v", err)
	}
	cache, err := itchio.LoadGamesCache(path)
	if err != nil {
		t.Fatalf("LoadGamesCache after overwrite: %v", err)
	}
	if cache.Games[0].Title != "New" {
		t.Errorf("got %q, want %q", cache.Games[0].Title, "New")
	}
	// Temp file must not linger.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("temp file .tmp should not exist after successful save")
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd /home/carroarmato0/Applications/Development/NextUI/Paks/Itch-io
go test ./internal/itchio/ -run "TestSaveAndLoadGamesCache|TestLoadGamesCache_MissingFile|TestLoadGamesCache_CorruptFile|TestSaveGamesCache_AtomicWrite" -v
```

Expected: compilation error (`itchio.SaveGamesCache` undefined).

- [ ] **Step 3: Implement `cache.go`**

```go
// internal/itchio/cache.go
package itchio

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// CacheMeta records when the cache was last populated.
type CacheMeta struct {
	FetchedAt  time.Time `json:"fetched_at"`
	TotalGames int       `json:"total_games"`
}

// GameCache is the on-disk representation of the full game list.
type GameCache struct {
	Meta  CacheMeta `json:"meta"`
	Games []Game    `json:"games"`
}

// SaveGamesCache writes games to path atomically (write to .tmp then rename).
func SaveGamesCache(path string, games []Game) error {
	cache := GameCache{
		Meta:  CacheMeta{FetchedAt: time.Now(), TotalGames: len(games)},
		Games: games,
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("marshal game cache: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write game cache tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename game cache: %w", err)
	}
	return nil
}

// LoadGamesCache reads and parses the cache file at path.
// Returns an error if the file is missing or unparseable.
func LoadGamesCache(path string) (*GameCache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read game cache: %w", err)
	}
	var cache GameCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("parse game cache: %w", err)
	}
	return &cache, nil
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./internal/itchio/ -run "TestSaveAndLoadGamesCache|TestLoadGamesCache_MissingFile|TestLoadGamesCache_CorruptFile|TestSaveGamesCache_AtomicWrite" -v
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Run full package tests to check for regressions**

```bash
go test ./internal/itchio/ -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/itchio/cache.go internal/itchio/cache_test.go
git commit -m "feat(cache): add SaveGamesCache and LoadGamesCache with atomic write"
```

---

### Task 2: `FetchAllGames` on the itchio Client (`internal/itchio/feed.go`)

**Files:**
- Modify: `internal/itchio/feed.go`
- Modify: `internal/itchio/feed_test.go`

Also fix `FetchGames` to use `c.base` instead of a hardcoded URL so both `FetchGames` and `FetchAllGames` are testable via `NewClientWithBase`.

- [ ] **Step 1: Write failing test**

Add to `internal/itchio/feed_test.go`:

```go
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
	games, err := c.FetchAllGames(context.Background(), func(fetched int) {
		progressCalls++
	})
	if err != nil {
		t.Fatalf("FetchAllGames: %v", err)
	}
	// rss_page1.xml has 36 items; page2 has 2 → total 38.
	if len(games) != 38 {
		t.Errorf("got %d games, want 38", len(games))
	}
	if progressCalls == 0 {
		t.Error("progress callback was never called")
	}
}

func TestFetchAllGames_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow server — real cancellation test.
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
```

Also add `"context"` to the imports in `feed_test.go`.

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/itchio/ -run "TestFetchAllGames" -v
```

Expected: compilation error (`FetchAllGames` undefined).

- [ ] **Step 3: Fix `FetchGames` to use `c.base` and add `FetchAllGames`**

In `internal/itchio/feed.go`, replace the `FetchGames` body and add `FetchAllGames`:

```go
// Replace the existing FetchGames body:
func (c *Client) FetchGames(page int, query string) ([]Game, error) {
	url := fmt.Sprintf("%s/games/made-with-gb-studio.xml?page=%d", c.base, page)
	if query != "" {
		url += "&q=" + query
	}
	return c.FetchGamesFromURL(url)
}
```

Add after `FetchGames` (add `"context"` and `"sync"` to the import block):

```go
// FetchAllGames fetches every page of the GB Studio feed until an empty page
// is returned or ctx is cancelled. progress is called with the running total
// of games fetched after each page (may be nil).
func (c *Client) FetchAllGames(ctx context.Context, progress func(fetched int)) ([]Game, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var (
		all []Game
		mu  sync.Mutex
	)

	// Fetch pages in parallel with a concurrency limit of 4, but only when we
	// know there are more pages to fetch. We discover bounds dynamically:
	// fetch page 1 first (sequential), then fan out over remaining pages.
	// This avoids sending unbounded requests to itch.io.

	fetchPage := func(page int) ([]Game, error) {
		url := fmt.Sprintf("%s/games/made-with-gb-studio.xml?page=%d", c.base, page)
		return c.FetchGamesFromURL(url)
	}

	// Page 1 — always first so we know whether there's anything to fetch.
	games, err := fetchPage(1)
	if err != nil {
		return nil, fmt.Errorf("fetch all games page 1: %w", err)
	}
	all = append(all, games...)
	if progress != nil {
		progress(len(all))
	}
	if len(games) < PerPage {
		return all, nil // single page, done
	}

	// Fetch remaining pages sequentially. Each page is fast (~1-2s); the user
	// is on the live feed during this background pass, so total time is acceptable.
	// Concurrency can be added later if profiling shows it matters.
	for page := 2; ; page++ {
		select {
		case <-ctx.Done():
			return all, ctx.Err()
		default:
		}

		games, err := fetchPage(page)
		if err != nil {
			logger.Warn("cache: page %d error: %v (stopping early)", page, err)
			break
		}
		mu.Lock()
		all = append(all, games...)
		total := len(all)
		mu.Unlock()
		if progress != nil {
			progress(total)
		}
		if len(games) < PerPage {
			break // last page
		}
	}
	return all, nil
}
```

Also add `"context"` and `"sync"` to the import block at the top of `feed.go`.

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./internal/itchio/ -run "TestFetchAllGames" -v
```

Expected: both `TestFetchAllGames` and `TestFetchAllGames_ContextCancellation` PASS.

- [ ] **Step 5: Run full package tests**

```bash
go test ./internal/itchio/ -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/itchio/feed.go internal/itchio/feed_test.go
git commit -m "feat(cache): add FetchAllGames; fix FetchGames to use c.base"
```

---

### Task 3: Wire cache into `ListScreen` (`internal/ui/screen_list.go`)

**Files:**
- Modify: `internal/ui/screen_list.go`

This is the core behavioural change. `ListScreen` gets three new fields, a new constructor parameter, and dual-mode `loadPage`.

- [ ] **Step 1: Add new fields and `cachePath` parameter to `ListScreen`**

In `screen_list.go`, add to the `ListScreen` struct (after the existing fields):

```go
	// Cache fields — populated once the on-disk game cache is loaded.
	// cachedGames is nil until the cache is available.
	cachedGames []itchio.Game
	cacheReady  bool
	cachePath   string
```

- [ ] **Step 2: Add `pageSlice` helper**

Add this function at the bottom of `screen_list.go` (alongside `truncateToWidth`):

```go
// pageSlice returns the sub-slice of games for the given 1-based page number,
// using the global PerPage constant.
func pageSlice(games []itchio.Game, page int) []itchio.Game {
	start := (page - 1) * itchio.PerPage
	if start >= len(games) {
		return nil
	}
	end := start + itchio.PerPage
	if end > len(games) {
		end = len(games)
	}
	return games[start:end]
}
```

- [ ] **Step 3: Rewrite `NewListScreen` to accept `cachePath` and implement dual-mode startup**

Replace the existing `NewListScreen` function:

```go
func NewListScreen(client *itchio.Client, cfg *settings.Config, cfgPath string, cache *renderer.ImageCache, cachePath string) *ListScreen {
	s := &ListScreen{
		client:    client,
		cfg:       cfg,
		cache:     cache,
		page:      1,
		cfgPath:   cfgPath,
		cachePath: cachePath,
	}

	gameCache, err := itchio.LoadGamesCache(cachePath)
	if err == nil && len(gameCache.Games) > 0 {
		// Cache hit: populate list instantly from disk.
		logger.Info("cache: loaded %d games from %s (age=%v)",
			len(gameCache.Games), cachePath, time.Since(gameCache.Meta.FetchedAt).Round(time.Second))
		s.cachedGames = gameCache.Games
		s.cacheReady = true
		s.totalGames = len(gameCache.Games)
		s.totalPages = (s.totalGames + itchio.PerPage - 1) / itchio.PerPage
		s.games = pageSlice(gameCache.Games, 1)
		// Refresh in background if stale.
		go s.refreshCacheIfStale(gameCache.Meta.FetchedAt)
	} else {
		// No cache: live fetch page 1 (existing behaviour) + build cache in background.
		if err != nil {
			logger.Debug("cache: no cache found (%v), using live feed", err)
		}
		go s.loadPage(1, "")
		go func() {
			total, err := client.FetchTotalGames()
			if err != nil {
				logger.Error("feed: total games: %v", err)
				return
			}
			logger.Info("feed: total games=%d", total)
			s.totalGames = total
			s.totalPages = (total + itchio.PerPage - 1) / itchio.PerPage
		}()
		go s.buildCache()
	}
	return s
}
```

- [ ] **Step 4: Rewrite `loadPage` to be dual-mode**

Replace the existing `loadPage` method:

```go
func (s *ListScreen) loadPage(page int, query string) {
	if s.cacheReady && query == "" {
		// Serve from local cache — no network, instant.
		logger.Debug("cache: serving page %d from cache (%d games)", page, len(s.cachedGames))
		s.games = pageSlice(s.cachedGames, page)
		s.cursor = 0
		s.titleScrollX = 0
		s.titleScrollAt = time.Now()
		return
	}
	// Live network fetch (existing behaviour).
	s.loading = true
	s.err = nil
	logger.Debug("feed: loading page %d query=%q", page, query)
	games, err := s.client.FetchGames(page, query)
	if err != nil {
		logger.Error("feed: page %d error: %v", page, err)
	} else {
		logger.Info("feed: page %d returned %d games", page, len(games))
	}
	s.games = games
	s.err = err
	s.cursor = 0
	s.titleScrollX = 0
	s.titleScrollAt = time.Now()
	s.loading = false
}
```

- [ ] **Step 5: Add `buildCache`, `refreshCacheIfStale`, and `triggerCacheRefresh`**

Add these methods to `screen_list.go`. Also add `"context"` to the imports.

```go
const cacheTTL = 24 * time.Hour

// buildCache fetches the complete game list and writes it to disk.
// Called as a goroutine. On success, future page turns use the local cache.
func (s *ListScreen) buildCache() {
	logger.Info("cache: starting background full fetch")
	games, err := s.client.FetchAllGames(context.Background(), func(fetched int) {
		logger.Debug("cache: fetched %d games so far", fetched)
	})
	if err != nil {
		logger.Error("cache: full fetch failed: %v", err)
		return
	}
	if err := itchio.SaveGamesCache(s.cachePath, games); err != nil {
		logger.Error("cache: save failed: %v", err)
		return
	}
	logger.Info("cache: saved %d games to %s", len(games), s.cachePath)
	// Flip to cache mode. The current page view is left untouched;
	// the next page navigation will source from the cache.
	s.cachedGames = games
	s.cacheReady = true
	s.totalGames = len(games)
	s.totalPages = (len(games) + itchio.PerPage - 1) / itchio.PerPage
}

// refreshCacheIfStale triggers a full re-fetch if the cache is older than cacheTTL.
func (s *ListScreen) refreshCacheIfStale(fetchedAt time.Time) {
	age := time.Since(fetchedAt)
	if age < cacheTTL {
		logger.Debug("cache: fresh (age=%v), skipping background refresh", age.Round(time.Second))
		return
	}
	logger.Info("cache: stale (age=%v), refreshing in background", age.Round(time.Second))
	s.buildCache()
}

// triggerCacheRefresh is the callback handed to SettingsScreen for the
// "Refresh Game List" menu item.
func (s *ListScreen) triggerCacheRefresh() {
	logger.Info("cache: manual refresh triggered from settings")
	go s.buildCache()
}
```

- [ ] **Step 6: Add `"context"` to imports in `screen_list.go`**

The import block should include:
```go
import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/carroarmato0/nextui-itchio-pak/internal/settings"
	"github.com/veandco/go-sdl2/sdl"
)
```

- [ ] **Step 7: Build to verify no compilation errors**

```bash
go build -tags headless ./...
```

Expected: clean build with no errors.

- [ ] **Step 8: Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "feat(cache): wire game cache into ListScreen with dual-mode loadPage"
```

---

### Task 4: "Refresh Game List" in Settings (`internal/ui/screen_settings.go`)

**Files:**
- Modify: `internal/ui/screen_settings.go`

- [ ] **Step 1: Add `onRefreshGames` callback field and `sItemRefreshCache` item**

In `screen_settings.go`, add the callback field to `SettingsScreen`:

```go
type SettingsScreen struct {
	cfg             *settings.Config
	cfgPath         string
	cursor          settingsItem
	prev            Screen
	onRefreshGames  func() // nil if not available

	heldDir    int
	heldSince  time.Time
	lastRepeat time.Time
}
```

Update the `settingsItem` constants — insert `sItemRefreshCache` between `sItemClearCache` and `sItemContentModeration`:

```go
const (
	sItemAPIKey settingsItem = iota
	sItemROMMode
	sItemROMLocation
	sItemLogLevel
	sItemClearCache
	sItemRefreshCache      // ← new
	sItemContentModeration
	sItemAbout
	sItemCount
)
```

- [ ] **Step 2: Update `NewSettingsScreen` signature**

```go
func NewSettingsScreen(cfg *settings.Config, cfgPath string, prev Screen, onRefreshGames func()) *SettingsScreen {
	return &SettingsScreen{cfg: cfg, cfgPath: cfgPath, prev: prev, onRefreshGames: onRefreshGames}
}
```

- [ ] **Step 3: Add "Refresh Game List" to the `items` slice in `Draw`**

Replace the `items` slice in `Draw`:

```go
	items := []string{
		"API Key: ",
		"ROM Selection: " + s.cfg.ROMSelection,
		"ROM Location: " + s.cfg.ROMLocation,
		"Log Level: " + logLevelLabel,
		"Clear Image Cache",
		"Refresh Game List",
		"Content Moderation >",
		"About",
	}
```

- [ ] **Step 4: Handle `sItemRefreshCache` in `activate`**

Add to the `switch s.cursor` in `activate()`:

```go
	case sItemRefreshCache:
		if s.onRefreshGames != nil {
			s.onRefreshGames()
		}
```

- [ ] **Step 5: Fix the two `NewSettingsScreen` call sites in `screen_list.go`**

Both `sdl.K_s` and `sdl.CONTROLLER_BUTTON_START` branches create a `SettingsScreen`. Update both:

```go
// keyboard:
case sdl.K_s:
    return NewSettingsScreen(s.cfg, s.cfgPath, s, s.triggerCacheRefresh)

// controller:
case sdl.CONTROLLER_BUTTON_START:
    return NewSettingsScreen(s.cfg, s.cfgPath, s, s.triggerCacheRefresh)
```

- [ ] **Step 6: Build to verify no compilation errors**

```bash
go build -tags headless ./...
```

Expected: clean build.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/screen_settings.go internal/ui/screen_list.go
git commit -m "feat(cache): add Refresh Game List to Settings"
```

---

### Task 5: Thread `cachePath` through `main_sdl.go`

**Files:**
- Modify: `cmd/itchio-pak/main_sdl.go`

- [ ] **Step 1: Derive `cachePath` and update `NewListScreen` call**

In `runSDL()`, after `cfgPath` is declared, add:

```go
cachePath := filepath.Join(filepath.Dir(cfgPath), "games_cache.json")
```

Then update the `NewListScreen` call:

```go
var current ui.Screen = ui.NewListScreen(client, cfg, cfgPath, cache, cachePath)
```

Add `"path/filepath"` to the import block if not already present (it is already imported in `main.go` in the same package; in `main_sdl.go` add it if needed — check the existing imports).

- [ ] **Step 2: Build the full binary**

```bash
go build -tags headless ./...
```

Expected: clean build.

- [ ] **Step 3: Run all tests**

```bash
go test ./... -v
```

Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/itchio-pak/main_sdl.go
git commit -m "feat(cache): pass cachePath to ListScreen from main_sdl"
```

---

## Self-Review

**Spec coverage check:**

| Requirement | Task |
|---|---|
| No cache → live feed (existing behaviour) | Task 3 (`NewListScreen` else branch) |
| Background full fetch while on live feed | Task 3 (`buildCache` goroutine) |
| Cache hit → instant list population | Task 3 (`NewListScreen` if branch) |
| Cache-aware page navigation (no network) | Task 3 (dual-mode `loadPage`) |
| Silent background refresh when stale (24h) | Task 3 (`refreshCacheIfStale`) |
| Drop `FetchTotalGames` scrape once cache is warm | Task 3 — `totalGames`/`totalPages` set from cache; `FetchTotalGames` goroutine only runs in no-cache path |
| Atomic cache write (no corrupt reads) | Task 1 (temp-file rename) |
| "Refresh Game List" in Settings | Task 4 |
| Cache stored in persistent userdata dir | Task 5 (derived from `cfgPath` dir) |
| `FetchAllGames` cancellable via context | Task 2 |

**Placeholder scan:** No TBDs, no "handle edge cases" without code, all test code is complete.

**Type consistency:**
- `itchio.GameCache`, `itchio.CacheMeta`, `itchio.SaveGamesCache`, `itchio.LoadGamesCache` — defined in Task 1, used in Tasks 3 & 5.
- `itchio.FetchAllGames(ctx context.Context, progress func(int)) ([]Game, error)` — defined in Task 2, called in Task 3.
- `NewListScreen(..., cachePath string)` — updated in Task 3, called in Task 5.
- `NewSettingsScreen(..., onRefreshGames func())` — updated in Task 4, called in Task 3 (two sites).
- `sItemRefreshCache` — added in Task 4 alongside `sItemCount` bump; `activate()` switch handles it.
- `pageSlice(games []itchio.Game, page int) []itchio.Game` — defined in Task 3, called in Tasks 3.
- `cacheTTL = 24 * time.Hour` — defined in Task 3, used in Task 3.
