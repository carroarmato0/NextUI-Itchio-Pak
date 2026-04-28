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
	result := itchio.ApplySort(games, itchio.SortModeRSS, nil, nil, nil)
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
	result := itchio.ApplySort(testGames(), itchio.SortModeAZ, nil, nil, nil)
	want := []string{"Apple", "Banana", "Cherry"}
	if !equalTitles(result, want) {
		t.Errorf("AZ: got %v, want %v", titles(result), want)
	}
}

func TestApplySort_ZA(t *testing.T) {
	result := itchio.ApplySort(testGames(), itchio.SortModeZA, nil, nil, nil)
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
	result := itchio.ApplySort(games, itchio.SortModeAZ, nil, nil, nil)
	want := []string{"Apple", "mango", "zebra"}
	if !equalTitles(result, want) {
		t.Errorf("AZ case-insensitive: got %v, want %v", titles(result), want)
	}
}

func TestApplySort_New(t *testing.T) {
	// Banana=2022 (newest), Cherry=2021, Apple=zero (sorts to end).
	result := itchio.ApplySort(testGames(), itchio.SortModeNew, nil, nil, nil)
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
	result := itchio.ApplySort(games, itchio.SortModeNew, nil, nil, nil)
	if len(result) != 2 {
		t.Fatalf("want 2 games, got %d", len(result))
	}
	// Stable sort must preserve input order when all dates are zero.
	if result[0].Title != "A" || result[1].Title != "B" {
		t.Errorf("NEW all-zero: want [A B], got %v", titles(result))
	}
}

func TestApplySort_DL_PendingUpdatesFirst(t *testing.T) {
	games := testGames()
	downloaded := map[string]bool{
		"https://a.itch.io/banana": true,
		"https://b.itch.io/apple":  true,
		"https://c.itch.io/cherry": true,
	}
	pendingUpdates := map[string]bool{
		"https://b.itch.io/apple": true,
	}
	result := itchio.ApplySort(games, itchio.SortModeDL, downloaded, pendingUpdates, nil)
	if len(result) != 3 {
		t.Fatalf("DL grouping: want 3 games, got %d", len(result))
	}
	if result[0].Title != "Apple" {
		t.Errorf("DL grouping: [UP] game should be first, got %q", result[0].Title)
	}
}

func TestApplySort_DL_RemovedSecond(t *testing.T) {
	games := testGames()
	downloaded := map[string]bool{
		"https://a.itch.io/banana": true,
		"https://b.itch.io/apple":  true,
		"https://c.itch.io/cherry": true,
	}
	removed := map[string]bool{
		"https://c.itch.io/cherry": true,
	}
	result := itchio.ApplySort(games, itchio.SortModeDL, downloaded, nil, removed)
	if len(result) != 3 {
		t.Fatalf("want 3 games, got %d", len(result))
	}
	if result[0].Title != "Cherry" {
		t.Errorf("DL grouping: [!] game should be first when no [UP] games, got %q", result[0].Title)
	}
}

func TestApplySort_DL_UpdateBeforeRemoved(t *testing.T) {
	games := testGames()
	downloaded := map[string]bool{
		"https://a.itch.io/banana": true,
		"https://b.itch.io/apple":  true,
		"https://c.itch.io/cherry": true,
	}
	pendingUpdates := map[string]bool{"https://b.itch.io/apple": true}
	removed := map[string]bool{"https://c.itch.io/cherry": true}
	result := itchio.ApplySort(games, itchio.SortModeDL, downloaded, pendingUpdates, removed)
	if result[0].Title != "Apple" {
		t.Errorf("want [UP] Apple first, got %q", result[0].Title)
	}
	if result[1].Title != "Cherry" {
		t.Errorf("want [!] Cherry second, got %q", result[1].Title)
	}
	if result[2].Title != "Banana" {
		t.Errorf("want [DL] Banana third, got %q", result[2].Title)
	}
}

func TestApplySort_DL(t *testing.T) {
	downloaded := map[string]bool{
		"https://a.itch.io/banana": true,
	}
	result := itchio.ApplySort(testGames(), itchio.SortModeDL, downloaded, nil, nil)
	if len(result) != 1 {
		t.Fatalf("DL: want 1 game, got %d", len(result))
	}
	if result[0].Title != "Banana" {
		t.Errorf("DL: got %q, want %q", result[0].Title, "Banana")
	}
}

func TestApplySort_DL_EmptyDownloaded(t *testing.T) {
	result := itchio.ApplySort(testGames(), itchio.SortModeDL, map[string]bool{}, nil, nil)
	if len(result) != 0 {
		t.Errorf("DL with empty downloaded: want 0 games, got %d", len(result))
	}
}

func TestApplySort_DL_NilDownloaded(t *testing.T) {
	result := itchio.ApplySort(testGames(), itchio.SortModeDL, nil, nil, nil)
	if len(result) != 0 {
		t.Errorf("DL with nil downloaded: want 0 games, got %d", len(result))
	}
}

func TestApplySort_Free(t *testing.T) {
	result := itchio.ApplySort(testGames(), itchio.SortModeFree, nil, nil, nil)
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
	result := itchio.ApplySort(testGames(), itchio.SortModePaid, nil, nil, nil)
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
		result := itchio.ApplySort(games, mode, nil, nil, nil)
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

func TestNextSortMode_UnknownModeFallsBackToRSS(t *testing.T) {
	got := itchio.NextSortMode("corrupt_value")
	if got != itchio.SortModeRSS {
		t.Errorf("unknown mode: got %q, want SortModeRSS", got)
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
