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
