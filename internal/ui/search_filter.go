package ui

import (
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

// applyPlatformFilter returns games whose Platform field matches platform
// (case-insensitive). An empty platform string returns all games unchanged.
func applyPlatformFilter(games []itchio.Game, platform string) []itchio.Game {
	if platform == "" {
		return games
	}
	out := make([]itchio.Game, 0, len(games))
	for _, g := range games {
		if strings.EqualFold(g.Platform, platform) {
			out = append(out, g)
		}
	}
	return out
}

// applySearchFilter returns games whose Title or Author contains query
// (case-insensitive). An empty query returns all games unchanged.
func applySearchFilter(games []itchio.Game, query string) []itchio.Game {
	if query == "" {
		return games
	}
	q := strings.ToLower(query)
	out := make([]itchio.Game, 0, len(games))
	for _, g := range games {
		if strings.Contains(strings.ToLower(g.Title), q) ||
			strings.Contains(strings.ToLower(g.Author), q) {
			out = append(out, g)
		}
	}
	return out
}
