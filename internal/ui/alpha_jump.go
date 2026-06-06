package ui

import "github.com/carroarmato0/nextui-itchio-pak/internal/itchio"

// alphaJumpIndex returns the index of the first game whose sort-key first rune
// differs from games[cursor]'s, scanning in direction dir (+1 = forward, -1 = backward).
// Returns the clamped list boundary if no new letter is found.
// Returns cursor unchanged when games is empty or cursor is out of range.
func alphaJumpIndex(games []itchio.Game, cursor, dir int) int {
	if len(games) == 0 || cursor < 0 || cursor >= len(games) {
		return cursor
	}
	curFirst := firstRune(itchio.SortKey(games[cursor].Title))
	i := cursor + dir
	for i >= 0 && i < len(games) {
		if firstRune(itchio.SortKey(games[i].Title)) != curFirst {
			return i
		}
		i += dir
	}
	// No boundary found: clamp to list edge.
	if dir > 0 {
		return len(games) - 1
	}
	return 0
}

// firstRune returns the first rune of s, or 0 if s is empty.
func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}
