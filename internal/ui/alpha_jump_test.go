package ui

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

var jumpGames = []itchio.Game{
	{Title: "Alwa's Awakening"},  // 0 → a
	{Title: "Arkade Boy"},        // 1 → a
	{Title: "Balloon Trip"},      // 2 → b
	{Title: "Byte Defender"},     // 3 → b
	{Title: "Cave Crawler"},      // 4 → c
	{Title: "Dino Quest"},        // 5 → d
}

func TestAlphaJumpForward(t *testing.T) {
	// From cursor=0 (a), forward should land at first 'b' (index 2)
	got := alphaJumpIndex(jumpGames, 0, 1)
	if got != 2 {
		t.Errorf("forward from 0: got %d, want 2", got)
	}
}

func TestAlphaJumpForwardMidLetter(t *testing.T) {
	// From cursor=1 (also 'a'), forward should land at first 'b' (index 2)
	got := alphaJumpIndex(jumpGames, 1, 1)
	if got != 2 {
		t.Errorf("forward from 1: got %d, want 2", got)
	}
}

func TestAlphaJumpBackward(t *testing.T) {
	// From cursor=3 (b), backward should land at index 1 (last 'a' before 'b')
	got := alphaJumpIndex(jumpGames, 3, -1)
	if got != 1 {
		t.Errorf("backward from 3: got %d, want 1", got)
	}
}

func TestAlphaJumpAtLastLetter(t *testing.T) {
	// From cursor=5 (d), forward clamps to last game
	got := alphaJumpIndex(jumpGames, 5, 1)
	if got != 5 {
		t.Errorf("forward from last letter: got %d, want 5 (clamped)", got)
	}
}

func TestAlphaJumpAtFirstLetter(t *testing.T) {
	// From cursor=0 (a), backward clamps to 0
	got := alphaJumpIndex(jumpGames, 0, -1)
	if got != 0 {
		t.Errorf("backward from first: got %d, want 0 (clamped)", got)
	}
}

func TestAlphaJumpEmptyList(t *testing.T) {
	got := alphaJumpIndex([]itchio.Game{}, 0, 1)
	if got != 0 {
		t.Errorf("empty list: got %d, want 0", got)
	}
}

func TestAlphaJumpSingleGame(t *testing.T) {
	games := []itchio.Game{{Title: "Solo"}}
	if got := alphaJumpIndex(games, 0, 1); got != 0 {
		t.Errorf("single game forward: got %d, want 0", got)
	}
	if got := alphaJumpIndex(games, 0, -1); got != 0 {
		t.Errorf("single game backward: got %d, want 0", got)
	}
}
