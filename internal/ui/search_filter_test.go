package ui

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

var filterTestGames = []itchio.Game{
	{Title: "Alwa's Awakening", Author: "Elden Pixels", Platform: "GB"},
	{Title: "Arkade Boy", Author: "Retro Dev", Platform: "GBC"},
	{Title: "Balloon Trip Remake", Author: "Balloon Dev", Platform: "GBC"},
	{Title: "Zelda Clone", Author: "Link Fan", Platform: "NES"},
}

func TestApplyPlatformFilter_Empty(t *testing.T) {
	out := applyPlatformFilter(filterTestGames, "")
	if len(out) != len(filterTestGames) {
		t.Errorf("empty filter: got %d games, want %d", len(out), len(filterTestGames))
	}
}

func TestApplyPlatformFilter_GBC(t *testing.T) {
	out := applyPlatformFilter(filterTestGames, "GBC")
	if len(out) != 2 {
		t.Fatalf("GBC filter: got %d games, want 2", len(out))
	}
	for _, g := range out {
		if g.Platform != "GBC" {
			t.Errorf("expected GBC game, got Platform=%q", g.Platform)
		}
	}
}

func TestApplyPlatformFilter_NoMatch(t *testing.T) {
	out := applyPlatformFilter(filterTestGames, "P8")
	if len(out) != 0 {
		t.Errorf("P8 filter: got %d games, want 0", len(out))
	}
}

func TestApplySearchFilter_Empty(t *testing.T) {
	out := applySearchFilter(filterTestGames, "")
	if len(out) != len(filterTestGames) {
		t.Errorf("empty search: got %d games, want %d", len(out), len(filterTestGames))
	}
}

func TestApplySearchFilter_TitleMatch(t *testing.T) {
	out := applySearchFilter(filterTestGames, "alwa")
	if len(out) != 1 || out[0].Title != "Alwa's Awakening" {
		t.Errorf("title search: unexpected result %v", out)
	}
}

func TestApplySearchFilter_AuthorMatch(t *testing.T) {
	out := applySearchFilter(filterTestGames, "Elden")
	if len(out) != 1 || out[0].Author != "Elden Pixels" {
		t.Errorf("author search: unexpected result %v", out)
	}
}

func TestApplySearchFilter_CaseInsensitive(t *testing.T) {
	out := applySearchFilter(filterTestGames, "ZELDA")
	if len(out) != 1 || out[0].Title != "Zelda Clone" {
		t.Errorf("case-insensitive search: unexpected result %v", out)
	}
}

func TestApplySearchFilter_NoMatch(t *testing.T) {
	out := applySearchFilter(filterTestGames, "xyzzy")
	if len(out) != 0 {
		t.Errorf("no-match search: got %d games, want 0", len(out))
	}
}
