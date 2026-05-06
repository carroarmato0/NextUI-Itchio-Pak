package itchio

import (
	"sort"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/text"
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

// SortModeBadge returns the display label shown in the UI header for m.
// An unrecognised mode is treated as SortModeRSS.
func SortModeBadge(m SortMode) string {
	switch m {
	case SortModeAZ:
		return "A-Z"
	case SortModeZA:
		return "Z-A"
	case SortModeNew:
		return "NEW"
	case SortModeDL:
		return "DL"
	case SortModeFree:
		return "FREE"
	case SortModePaid:
		return "PAID"
	default:
		return "RSS"
	}
}

// NextSortMode returns the mode that follows current in the SortModes cycle.
// If current is not a recognised mode it returns SortModeRSS.
func NextSortMode(current SortMode) SortMode {
	for i, m := range SortModes {
		if m == current {
			return SortModes[(i+1)%len(SortModes)]
		}
	}
	return SortModeRSS // treat any unrecognised value as RSS; next press goes to AZ
}

// sortKey returns a normalised sort key for a game title: emoji stripped, whitespace trimmed, lowercased.
func sortKey(s string) string {
	return strings.ToLower(strings.TrimSpace(text.StripEmoji(s)))
}

// SortKey is the exported form of sortKey, used by UI layers for alpha-jump navigation.
func SortKey(s string) string { return sortKey(s) }

// ApplySort returns a new slice derived from games according to mode.
// downloaded maps game URLs to true when present in the inventory.
// pendingUpdates and removed map URLs to true for [UP]/[!] grouping in DL mode.
// games is never mutated.
func ApplySort(games []Game, mode SortMode, downloaded, pendingUpdates, removed map[string]bool) []Game {
	switch mode {
	case SortModeAZ:
		out := make([]Game, len(games))
		copy(out, games)
		sort.SliceStable(out, func(i, j int) bool {
			return sortKey(out[i].Title) < sortKey(out[j].Title)
		})
		return out

	case SortModeZA:
		out := make([]Game, len(games))
		copy(out, games)
		sort.SliceStable(out, func(i, j int) bool {
			return sortKey(out[i].Title) > sortKey(out[j].Title)
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
		// Collect downloaded games then sort into three groups:
		// 1 — pending updates, 2 — removed from store, 3 — up-to-date
		var g1, g2, g3 []Game
		for _, g := range games {
			if !downloaded[g.URL] {
				continue
			}
			switch {
			case pendingUpdates[g.URL]:
				g1 = append(g1, g)
			case removed[g.URL]:
				g2 = append(g2, g)
			default:
				g3 = append(g3, g)
			}
		}
		out := make([]Game, 0, len(g1)+len(g2)+len(g3))
		out = append(out, g1...)
		out = append(out, g2...)
		out = append(out, g3...)
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
