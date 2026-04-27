# Sort & Filter System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a SELECT-cycled sort/filter system to the game list screen, persisted in config.json, with a coloured header badge and empty-state messaging.

**Architecture:** Pure sort logic lives in a new `internal/itchio/sort.go` (no SDL2 dependency, fully unit-testable). The `ListScreen` holds a `viewGames` derived slice that drives paging; `rebuildView()` regenerates it whenever the sort mode or underlying cache changes. The active mode is persisted to `Config.SortMode` on each SELECT press.

**Tech Stack:** Go 1.22, SDL2 (UI layer only), no new dependencies.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/itchio/feed.go` | Modify | Add `PublishedAt time.Time` to `Game`; add `PubDate string` to `rssItem`; add `parsePubDate` helper |
| `internal/itchio/feed_test.go` | Modify | Add test verifying `PublishedAt` is populated from `pubDate` XML field |
| `internal/itchio/sort.go` | Create | `SortMode` type + constants, `SortModes` cycle slice, `SortModeBadge`, `NextSortMode`, `ApplySort` |
| `internal/itchio/sort_test.go` | Create | Unit tests for all sort/filter modes, cycle order, edge cases |
| `internal/settings/settings.go` | Modify | Add `SortMode string` to `Config` |
| `internal/settings/settings_test.go` | Modify | Tests for SortMode round-trip and omitempty behaviour |
| `internal/ui/screen_list.go` | Modify | Add `sortMode`, `viewGames` fields; `rebuildView()`; SELECT handler; badge + empty-state + footer in `Draw` |

---

## Task 1: Add `PublishedAt` to `Game` and parse `pubDate`

**Files:**
- Modify: `internal/itchio/feed.go`
- Modify: `internal/itchio/feed_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/itchio/feed_test.go` (after the existing imports, add `"time"` to the import block):

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/carroarmato0/Applications/Development/NextUI/Paks/Itch-io
go test -race -tags headless ./internal/itchio/... -run TestFetchGamesFromURL_PublishedAt
```

Expected: compile error — `games[0].PublishedAt` field does not exist.

- [ ] **Step 3: Add `PublishedAt` to `Game` in `internal/itchio/feed.go`**

The `Game` struct starts at line 16. Add the import `"time"` to the import block, then add the field:

```go
// in the import block add:
"time"

// Game struct — replace the existing struct with:
type Game struct {
	Title       string    `json:"title"`
	Author      string    `json:"author"`
	URL         string    `json:"url"`
	CoverURL    string    `json:"cover_url"`
	Price       float64   `json:"price"`
	IsFree      bool      `json:"is_free"`
	Tags        []string  `json:"tags,omitempty"`
	PublishedAt time.Time `json:"published_at,omitempty"`
}
```

- [ ] **Step 4: Add `PubDate` to `rssItem` and the `parsePubDate` helper**

In `internal/itchio/feed.go`, update the `rssItem` struct (currently at line 51):

```go
type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	ImageURL    string `xml:"imageurl"`
	Price       string `xml:"price"`
	PubDate     string `xml:"pubDate"`
}
```

Add the `parsePubDate` helper after `parsePrice`:

```go
func parsePubDate(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC1123, time.RFC1123Z} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}
```

- [ ] **Step 5: Wire `PublishedAt` in `FetchGamesFromURL`**

In `internal/itchio/feed.go`, update the game-building loop in `FetchGamesFromURL` (currently around line 124):

```go
	games := make([]Game, 0, len(feed.Items))
	for _, item := range feed.Items {
		price := parsePrice(item.Price)
		games = append(games, Game{
			Title:       parseTitle(item.Title),
			Tags:        parseTags(item.Title),
			Author:      parseAuthor(item.Link),
			URL:         item.Link,
			CoverURL:    parseCover(item.ImageURL, item.Description),
			Price:       price,
			IsFree:      price == 0,
			PublishedAt: parsePubDate(item.PubDate),
		})
	}
```

- [ ] **Step 6: Run test to verify it passes**

```bash
go test -race -tags headless ./internal/itchio/... -run TestFetchGamesFromURL_PublishedAt
```

Expected: PASS

- [ ] **Step 7: Run full test suite to check for regressions**

```bash
go test -race -tags headless ./...
```

Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add internal/itchio/feed.go internal/itchio/feed_test.go
git commit -m "feat(feed): add PublishedAt field to Game, parse pubDate from RSS"
```

---

## Task 2: Add `SortMode` to `Config`

**Files:**
- Modify: `internal/settings/settings.go`
- Modify: `internal/settings/settings_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/settings/settings_test.go`:

```go
func TestSortModeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{SortMode: "az"}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.SortMode != "az" {
		t.Errorf("SortMode = %q, want %q", loaded.SortMode, "az")
	}
}

func TestSortModeDefaultsToEmpty(t *testing.T) {
	cfg, err := settings.Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SortMode != "" {
		t.Errorf("default SortMode = %q, want %q", cfg.SortMode, "")
	}
}

func TestSortModeOmittedWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &settings.Config{} // SortMode is ""
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.Contains(data, []byte("sort_mode")) {
		t.Errorf("sort_mode should be omitted when empty, found in JSON:\n%s", data)
	}
}

func TestSortModeBackwardsCompatible(t *testing.T) {
	// Old config without sort_mode key must unmarshal to ""
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	oldJSON := `{"api_key":"","rom_selection":"auto","content_filter":{}}`
	if err := os.WriteFile(path, []byte(oldJSON), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.SortMode != "" {
		t.Errorf("old config SortMode = %q, want empty string", loaded.SortMode)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -race -tags headless ./internal/settings/... -run "TestSortMode"
```

Expected: compile error — `settings.Config` has no field `SortMode`.

- [ ] **Step 3: Add `SortMode` to `Config` in `internal/settings/settings.go`**

Add the field to `Config` (after `LogLevel`):

```go
type Config struct {
	APIKey       string            `json:"api_key"`
	ROMSelection string            `json:"rom_selection"`
	ROMLocation  string            `json:"rom_location"`
	LastROMDirs  map[string]string `json:"last_rom_dirs,omitempty"`
	Filter       ContentFilter     `json:"content_filter"`
	LogLevel     string            `json:"log_level,omitempty"`
	SortMode     string            `json:"sort_mode,omitempty"`
}
```

`defaults()` is unchanged — `SortMode` zero-value `""` maps to `[RSS]` at runtime.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -race -tags headless ./internal/settings/... -run "TestSortMode"
```

Expected: all 4 PASS

- [ ] **Step 5: Run full test suite**

```bash
go test -race -tags headless ./...
```

Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/settings/settings.go internal/settings/settings_test.go
git commit -m "feat(settings): add SortMode field to Config"
```

---

## Task 3: Create `sort.go` with sort/filter logic

**Files:**
- Create: `internal/itchio/sort.go`
- Create: `internal/itchio/sort_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/itchio/sort_test.go`:

```go
package itchio_test

import (
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

// testGames returns a fixed slice for deterministic tests.
// Titles: "Banana", "Apple", "Cherry" (intentionally unsorted).
// PublishedAt: Banana=2022, Apple=zero, Cherry=2021.
func testGames() []itchio.Game {
	return []itchio.Game{
		{Title: "Banana", URL: "https://a.itch.io/banana", IsFree: false, Price: 5.00,
			PublishedAt: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Title: "Apple", URL: "https://b.itch.io/apple", IsFree: true,
			PublishedAt: time.Time{}},
		{Title: "Cherry", URL: "https://c.itch.io/cherry", IsFree: true,
			PublishedAt: time.Date(2021, 6, 15, 0, 0, 0, 0, time.UTC)},
	}
}

func TestApplySort_RSS(t *testing.T) {
	games := testGames()
	result := itchio.ApplySort(games, itchio.SortModeRSS, nil)
	if len(result) != 3 {
		t.Fatalf("want 3 games, got %d", len(result))
	}
	if result[0].Title != "Banana" || result[1].Title != "Apple" || result[2].Title != "Cherry" {
		t.Errorf("RSS mode must preserve original order, got %v", titles(result))
	}
	// Must be a copy — not the same backing array.
	result[0].Title = "MUTATED"
	if games[0].Title == "MUTATED" {
		t.Error("ApplySort must not share backing array with input")
	}
}

func TestApplySort_AZ(t *testing.T) {
	result := itchio.ApplySort(testGames(), itchio.SortModeAZ, nil)
	want := []string{"Apple", "Banana", "Cherry"}
	if !equalTitles(result, want) {
		t.Errorf("AZ: got %v, want %v", titles(result), want)
	}
}

func TestApplySort_ZA(t *testing.T) {
	result := itchio.ApplySort(testGames(), itchio.SortModeZA, nil)
	want := []string{"Cherry", "Banana", "Apple"}
	if !equalTitles(result, want) {
		t.Errorf("ZA: got %v, want %v", titles(result), want)
	}
}

func TestApplySort_AZ_CaseInsensitive(t *testing.T) {
	games := []itchio.Game{
		{Title: "zebra"},
		{Title: "Apple"},
		{Title: "mango"},
	}
	result := itchio.ApplySort(games, itchio.SortModeAZ, nil)
	want := []string{"Apple", "mango", "zebra"}
	if !equalTitles(result, want) {
		t.Errorf("AZ case-insensitive: got %v, want %v", titles(result), want)
	}
}

func TestApplySort_New(t *testing.T) {
	// Banana=2022 (newest), Cherry=2021, Apple=zero (sorts to end).
	result := itchio.ApplySort(testGames(), itchio.SortModeNew, nil)
	want := []string{"Banana", "Cherry", "Apple"}
	if !equalTitles(result, want) {
		t.Errorf("NEW: got %v, want %v", titles(result), want)
	}
}

func TestApplySort_New_AllZero(t *testing.T) {
	games := []itchio.Game{
		{Title: "A", PublishedAt: time.Time{}},
		{Title: "B", PublishedAt: time.Time{}},
	}
	result := itchio.ApplySort(games, itchio.SortModeNew, nil)
	if len(result) != 2 {
		t.Fatalf("want 2 games, got %d", len(result))
	}
}

func TestApplySort_DL(t *testing.T) {
	downloaded := map[string]bool{
		"https://a.itch.io/banana": true,
	}
	result := itchio.ApplySort(testGames(), itchio.SortModeDL, downloaded)
	if len(result) != 1 {
		t.Fatalf("DL: want 1 game, got %d", len(result))
	}
	if result[0].Title != "Banana" {
		t.Errorf("DL: got %q, want %q", result[0].Title, "Banana")
	}
}

func TestApplySort_DL_EmptyDownloaded(t *testing.T) {
	result := itchio.ApplySort(testGames(), itchio.SortModeDL, map[string]bool{})
	if len(result) != 0 {
		t.Errorf("DL with empty downloaded: want 0 games, got %d", len(result))
	}
}

func TestApplySort_DL_NilDownloaded(t *testing.T) {
	result := itchio.ApplySort(testGames(), itchio.SortModeDL, nil)
	if len(result) != 0 {
		t.Errorf("DL with nil downloaded: want 0 games, got %d", len(result))
	}
}

func TestApplySort_Free(t *testing.T) {
	result := itchio.ApplySort(testGames(), itchio.SortModeFree, nil)
	if len(result) != 2 {
		t.Fatalf("FREE: want 2 games, got %d", len(result))
	}
	for _, g := range result {
		if !g.IsFree {
			t.Errorf("FREE filter returned non-free game %q", g.Title)
		}
	}
}

func TestApplySort_Paid(t *testing.T) {
	result := itchio.ApplySort(testGames(), itchio.SortModePaid, nil)
	if len(result) != 1 {
		t.Fatalf("PAID: want 1 game, got %d", len(result))
	}
	if result[0].IsFree {
		t.Errorf("PAID filter returned free game %q", result[0].Title)
	}
}

func TestApplySort_ReturnsNewSlice(t *testing.T) {
	games := testGames()
	for _, mode := range itchio.SortModes {
		result := itchio.ApplySort(games, mode, nil)
		if result == nil {
			t.Errorf("mode %q: ApplySort returned nil", mode)
		}
	}
}

func TestSortModeBadge(t *testing.T) {
	cases := []struct {
		mode itchio.SortMode
		want string
	}{
		{itchio.SortModeRSS, "[RSS]"},
		{itchio.SortModeAZ, "[A-Z]"},
		{itchio.SortModeZA, "[Z-A]"},
		{itchio.SortModeNew, "[NEW]"},
		{itchio.SortModeDL, "[DL]"},
		{itchio.SortModeFree, "[FREE]"},
		{itchio.SortModePaid, "[PAID]"},
	}
	for _, tc := range cases {
		got := itchio.SortModeBadge(tc.mode)
		if got != tc.want {
			t.Errorf("SortModeBadge(%q) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

func TestNextSortMode_Cycle(t *testing.T) {
	// Full cycle must return to RSS after PAID.
	mode := itchio.SortModeRSS
	seen := make([]itchio.SortMode, 0, len(itchio.SortModes))
	for i := 0; i < len(itchio.SortModes); i++ {
		mode = itchio.NextSortMode(mode)
		seen = append(seen, mode)
	}
	// After len(SortModes) presses, we should be back at RSS.
	if mode != itchio.SortModeRSS {
		t.Errorf("cycle: after %d presses expected RSS, got %q", len(itchio.SortModes), mode)
	}
	// Every mode must appear exactly once.
	for _, m := range itchio.SortModes {
		count := 0
		for _, s := range seen {
			if s == m {
				count++
			}
		}
		if count != 1 {
			t.Errorf("mode %q appeared %d times in cycle (want 1)", m, count)
		}
	}
}

// helpers

func titles(games []itchio.Game) []string {
	out := make([]string, len(games))
	for i, g := range games {
		out[i] = g.Title
	}
	return out
}

func equalTitles(games []itchio.Game, want []string) bool {
	if len(games) != len(want) {
		return false
	}
	for i, g := range games {
		if g.Title != want[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -race -tags headless ./internal/itchio/... -run "TestApplySort|TestSortMode|TestNextSortMode"
```

Expected: compile error — `itchio.SortModeRSS`, `itchio.ApplySort`, etc. are undefined.

- [ ] **Step 3: Create `internal/itchio/sort.go`**

```go
package itchio

import (
	"sort"
	"strings"
)

type SortMode string

const (
	SortModeRSS  SortMode = ""
	SortModeAZ   SortMode = "az"
	SortModeZA   SortMode = "za"
	SortModeNew  SortMode = "new"
	SortModeDL   SortMode = "dl"
	SortModeFree SortMode = "free"
	SortModePaid SortMode = "paid"
)

var SortModes = []SortMode{
	SortModeRSS, SortModeAZ, SortModeZA, SortModeNew,
	SortModeDL, SortModeFree, SortModePaid,
}

func SortModeBadge(m SortMode) string {
	switch m {
	case SortModeAZ:
		return "[A-Z]"
	case SortModeZA:
		return "[Z-A]"
	case SortModeNew:
		return "[NEW]"
	case SortModeDL:
		return "[DL]"
	case SortModeFree:
		return "[FREE]"
	case SortModePaid:
		return "[PAID]"
	default:
		return "[RSS]"
	}
}

func NextSortMode(current SortMode) SortMode {
	for i, m := range SortModes {
		if m == current {
			return SortModes[(i+1)%len(SortModes)]
		}
	}
	return SortModeAZ
}

// ApplySort returns a new slice derived from games according to mode.
// downloaded maps game URLs to true when present in the inventory.
// games is never mutated.
func ApplySort(games []Game, mode SortMode, downloaded map[string]bool) []Game {
	switch mode {
	case SortModeAZ:
		out := make([]Game, len(games))
		copy(out, games)
		sort.SliceStable(out, func(i, j int) bool {
			return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
		})
		return out

	case SortModeZA:
		out := make([]Game, len(games))
		copy(out, games)
		sort.SliceStable(out, func(i, j int) bool {
			return strings.ToLower(out[i].Title) > strings.ToLower(out[j].Title)
		})
		return out

	case SortModeNew:
		out := make([]Game, len(games))
		copy(out, games)
		sort.SliceStable(out, func(i, j int) bool {
			ti, tj := out[i].PublishedAt, out[j].PublishedAt
			if ti.IsZero() {
				return false
			}
			if tj.IsZero() {
				return true
			}
			return ti.After(tj)
		})
		return out

	case SortModeDL:
		out := make([]Game, 0)
		for _, g := range games {
			if downloaded[g.URL] {
				out = append(out, g)
			}
		}
		return out

	case SortModeFree:
		out := make([]Game, 0)
		for _, g := range games {
			if g.IsFree {
				out = append(out, g)
			}
		}
		return out

	case SortModePaid:
		out := make([]Game, 0)
		for _, g := range games {
			if !g.IsFree {
				out = append(out, g)
			}
		}
		return out

	default: // SortModeRSS
		out := make([]Game, len(games))
		copy(out, games)
		return out
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -race -tags headless ./internal/itchio/... -run "TestApplySort|TestSortMode|TestNextSortMode"
```

Expected: all PASS

- [ ] **Step 5: Run full test suite**

```bash
go test -race -tags headless ./...
```

Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/itchio/sort.go internal/itchio/sort_test.go
git commit -m "feat(itchio): add sort/filter logic — ApplySort, SortModeBadge, NextSortMode"
```

---

## Task 4: Wire sort into `ListScreen`

**Files:**
- Modify: `internal/ui/screen_list.go`

This file has `//go:build !headless` so it is excluded from the unit-test suite. Correctness is verified by `scripts/build.sh` (full cross-compile).

### 4a: Add fields and `rebuildView`

- [ ] **Step 1: Add `sortMode` and `viewGames` to `ListScreen`**

In `internal/ui/screen_list.go`, add two fields to the `ListScreen` struct (after `jumpToEnd`):

```go
	// Sort/filter state
	sortMode  itchio.SortMode
	viewGames []itchio.Game // sorted/filtered view; paging operates on this
```

- [ ] **Step 2: Add `rebuildView` method**

Add after the `newCacheRefreshScreen` method at the end of the file:

```go
// rebuildView regenerates viewGames from cachedGames using the current sortMode,
// then resets paging to page 1.
func (s *ListScreen) rebuildView() {
	downloaded := make(map[string]bool)
	for _, g := range s.cachedGames {
		if s.inv.IsPresent(g.URL) {
			downloaded[g.URL] = true
		}
	}
	s.viewGames = itchio.ApplySort(s.cachedGames, s.sortMode, downloaded)
	s.totalGames = len(s.viewGames)
	s.totalPages = (s.totalGames + itchio.PerPage - 1) / itchio.PerPage
	s.page = 1
	s.loadPage(1, "")
}
```

### 4b: Update `NewListScreen` to load sortMode and call `rebuildView`

- [ ] **Step 3: Load `sortMode` from config in `NewListScreen`**

In `NewListScreen`, after the struct literal initialisation of `s` (around line 79), add:

```go
	s.sortMode = itchio.SortMode(cfg.SortMode)
```

- [ ] **Step 4: Replace direct cache wiring with `rebuildView` in the cache-hit branch**

In `NewListScreen`, replace the cache-hit branch (lines ~83–91):

```go
	// Before (remove these lines):
	s.cachedGames = gameCache.Games
	s.cacheReady = true
	s.totalGames = len(gameCache.Games)
	s.totalPages = (s.totalGames + itchio.PerPage - 1) / itchio.PerPage
	s.games = pageSlice(gameCache.Games, 1)
	// Refresh in background if stale.
	go s.refreshCacheIfStale(gameCache.Meta.FetchedAt)

	// After (replace with):
	s.cachedGames = gameCache.Games
	s.cacheReady = true
	s.rebuildView()
	// Refresh in background if stale.
	go s.refreshCacheIfStale(gameCache.Meta.FetchedAt)
```

### 4c: Update `loadPage` to serve from `viewGames`

- [ ] **Step 5: Replace `cachedGames` with `viewGames` in the cache-ready branch of `loadPage`**

In `loadPage` (around line 118), replace:

```go
	// Before:
	logger.Debug("cache: serving page %d from cache (%d games)", page, len(s.cachedGames))
	s.games = pageSlice(s.cachedGames, page)

	// After:
	logger.Debug("cache: serving page %d from view (%d games)", page, len(s.viewGames))
	s.games = pageSlice(s.viewGames, page)
```

### 4d: Simplify `buildCache` and `newCacheRefreshScreen` callbacks

- [ ] **Step 6: Replace manual cache-wiring in `buildCache` with `rebuildView`**

In `buildCache` (around line 613), replace the block after the SaveGamesCache call:

```go
	// Before (remove these lines):
	s.cachedGames = games
	s.cacheReady = true
	s.totalGames = len(games)
	s.totalPages = (len(games) + itchio.PerPage - 1) / itchio.PerPage
	if s.page > s.totalPages {
		logger.Info("cache: current page %d exceeds new total %d, jumping to last page", s.page, s.totalPages)
		s.page = s.totalPages
		s.loadPage(s.page, "")
	}

	// After (replace with):
	s.cachedGames = games
	s.cacheReady = true
	s.rebuildView()
```

- [ ] **Step 7: Replace manual cache-wiring in `newCacheRefreshScreen` callback with `rebuildView`**

In `newCacheRefreshScreen` (around line 642), the callback passed to `NewCacheRefreshScreen`:

```go
	// Before (remove these lines):
	func(games []itchio.Game) {
		s.cachedGames = games
		s.cacheReady = true
		s.totalGames = len(games)
		s.totalPages = (len(games) + itchio.PerPage - 1) / itchio.PerPage
		if s.page > s.totalPages {
			logger.Info("cache: current page %d exceeds new total %d, jumping to last page", s.page, s.totalPages)
			s.page = s.totalPages
			s.loadPage(s.page, "")
		}
	}

	// After (replace with):
	func(games []itchio.Game) {
		s.cachedGames = games
		s.cacheReady = true
		s.rebuildView()
	}
```

### 4e: Add SELECT handler

- [ ] **Step 8: Add the SELECT (CONTROLLER_BUTTON_BACK) case to `HandleEvent`**

In the `ControllerButtonEvent` switch inside `HandleEvent`, add after the existing cases (e.g., after the `sdl.CONTROLLER_BUTTON_START` case, before the closing brace of the outer switch):

```go
		case sdl.CONTROLLER_BUTTON_BACK:
			if !s.cacheReady {
				return s
			}
			s.sortMode = itchio.NextSortMode(s.sortMode)
			logger.Debug("sort: mode changed to %q (%s)", s.sortMode, itchio.SortModeBadge(s.sortMode))
			s.rebuildView()
			s.cfg.SortMode = string(s.sortMode)
			go s.cfg.Save(s.cfgPath)
			return s
```

### 4f: Update `Draw` — header badge

- [ ] **Step 9: Add the sort-mode badge to the header in `Draw`**

In `Draw`, find the header section. Replace the existing title draw line:

```go
	// Before:
	r.DrawText("Itch.io — GB Studio Games", 12, (headerH-fontH)/2, colorText, colorText, colorText)

	// After:
	headerTextY := (headerH - fontH) / 2
	r.DrawText("Itch.io — GB Studio Games", 12, headerTextY, colorText, colorText, colorText)
	if s.cacheReady {
		badge := itchio.SortModeBadge(s.sortMode)
		bw, _ := r.TextSize(badge)
		bx := r.W - bw - 12
		var badgeR, badgeG, badgeB uint8
		switch s.sortMode {
		case itchio.SortModeFree:
			badgeR, badgeG, badgeB = 80, 200, 80
		case itchio.SortModePaid:
			badgeR, badgeG, badgeB = 220, 180, 60
		default:
			badgeR, badgeG, badgeB = 80, 200, 220
		}
		r.DrawText(badge, bx, headerTextY, badgeR, badgeG, badgeB)
	}
```

### 4g: Update `Draw` — empty state

- [ ] **Step 10: Add empty-state rendering after layout variables are defined**

In `Draw`, find the block where `leftW`, `footerH`, and the list-rendering variables are defined (around line 231). Add the empty-state check immediately after `footerH` and before the `startIdx` calculation:

```go
	// Add after:  footerH := int32(40)
	// Add before: startIdx := 0

	if len(s.viewGames) == 0 && s.cacheReady {
		r.DrawTextCentered("No games match this filter.", 0, r.H/2-fontH, leftW, 140, 140, 140)
		r.DrawTextCentered("Press SELECT to change sort.", 0, r.H/2+4, leftW, 80, 160, 180)
		ftrY := r.DrawFooterBar(footerH)
		r.DrawSmallText("SELECT:sort  B:exit  Start:settings", 10, ftrY, 140, 140, 140)
		r.Present()
		return
	}
```

### 4h: Update `Draw` — footer hint

- [ ] **Step 11: Add `SELECT:sort` to the footer hint when `cacheReady`**

Find the footer section near the bottom of `Draw` (around line 421). Replace:

```go
	// Before:
	footer := fmt.Sprintf("%s · %s  |  A:select  L/R:page  B:exit  Start:settings", pageInfo, countInfo)

	// After:
	var hints string
	if s.cacheReady {
		hints = "A:select  L/R:page  SELECT:sort  B:exit  Start:settings"
	} else {
		hints = "A:select  L/R:page  B:exit  Start:settings"
	}
	footer := fmt.Sprintf("%s · %s  |  %s", pageInfo, countInfo, hints)
```

### 4i: Verify and commit

- [ ] **Step 12: Verify headless tests still pass**

```bash
go test -race -tags headless ./...
```

Expected: all PASS (screen_list.go is excluded by the build tag, so this checks nothing regressed in the pure-Go packages)

- [ ] **Step 13: Verify full SDL2 build compiles**

```bash
scripts/build.sh
```

Expected: build succeeds for all three platform targets with no compile errors.

- [ ] **Step 14: Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "feat(ui): add sort/filter system — SELECT cycles modes, badge in header, empty state"
```

---

## Self-Review Against Spec

| Spec Requirement | Task |
|-----------------|------|
| `PublishedAt time.Time` in `Game` | Task 1 |
| `PubDate` parsed from RSS (RFC1123/RFC1123Z) | Task 1 |
| `SortMode string` in `Config` with omitempty | Task 2 |
| Old configs unmarshal to `""` | Task 2 |
| `SortMode` type + 7 constants | Task 3 |
| `SortModes` cycle slice | Task 3 |
| `SortModeBadge` correct strings | Task 3 |
| `NextSortMode` wraps RSS→AZ→…→PAID→RSS | Task 3 |
| `ApplySort` never mutates input | Task 3 |
| `ApplySort` DL with nil/empty downloaded → empty slice (not nil) | Task 3 |
| `[NEW]` zero dates sort to end | Task 3 |
| `viewGames` field drives paging | Task 4 |
| `rebuildView()` resets page=1 | Task 4 |
| `loadPage` serves from `viewGames` when cacheReady | Task 4 |
| `rebuildView` called on cache load + SELECT + cache refresh | Task 4 |
| `sortMode` loaded from cfg in `NewListScreen` | Task 4 |
| SELECT ignored when `!cacheReady` | Task 4 |
| SELECT saves to config async | Task 4 |
| Badge hidden when `!cacheReady` | Task 4 |
| Badge colours: teal/green/gold per spec | Task 4 |
| Empty state text + SELECT still works | Task 4 |
| Footer `SELECT:sort` when cacheReady | Task 4 |
