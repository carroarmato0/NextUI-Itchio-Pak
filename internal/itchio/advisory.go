package itchio

import (
	"slices"
	"strings"
)

// CategoryFilter holds the enabled state and individually-disabled tags for
// one content filter category. Disabled is an opt-out list: tags in this list
// are excluded from filtering even when Enabled is true.
type CategoryFilter struct {
	Enabled  bool
	Disabled []string
}

// FilterConfig is the complete content filter configuration passed to
// IsAdvisoryTriggered. It lives in the itchio package (not settings) to avoid
// import cycles — callers in the ui package convert from settings.ContentFilter.
type FilterConfig struct {
	AdultContent CategoryFilter
	QueerContent CategoryFilter
	HeavyThemes  CategoryFilter
	SubstanceUse CategoryFilter
}

// AdultContentTags covers explicit adult content and suggestive content
// (previously split across MatureTags and SexualContentTags).
// Users can toggle the category and opt individual tags in or out.
// Sorted alphabetically.
var AdultContentTags = []string{
	"adult", "boobs", "ecchi", "eroge", "erotic", "femdom", "gore",
	"hentai", "innuendo", "lewd", "nsfw", "nudity", "porn",
	"sexual-content", "sexy", "softcore", "suggestive", "tits", "titties",
	"xxx", "yaoi", "yuri",
}

// QueerContentTags covers LGBTQ+ themes and representation.
// Users can toggle the category and opt individual tags in or out.
// Sorted alphabetically.
var QueerContentTags = []string{
	"achillean", "aromantic", "asexual", "bisexual", "enby",
	"gay", "gender", "intersex", "lesbian", "lgbt", "lgbtq",
	"lgbtqia", "mlm", "non-binary", "nonbinary", "pansexual",
	"pride", "queer", "sapphic", "trans", "transgender", "wlw",
}

// HeavyThemesTags covers potentially distressing narrative themes.
// Users can toggle the category and opt individual tags in or out.
// Sorted alphabetically.
var HeavyThemesTags = []string{
	"abuse", "anxiety", "bereavement", "child-loss", "death",
	"depression", "domestic-abuse", "eating-disorder", "grief",
	"loss", "mental-health", "mental-illness", "miscarriage",
	"self-harm", "sexual-assault", "suicide", "trauma", "war",
}

// SubstanceUseTags covers drug and alcohol themes.
var SubstanceUseTags = []string{
	"addiction", "alcohol", "drug-use", "drugs", "substance-abuse",
}

// normalizeTagList returns a copy of list with each element lowercased and trimmed.
func normalizeTagList(list []string) []string {
	out := make([]string, len(list))
	for i, d := range list {
		out[i] = strings.ToLower(strings.TrimSpace(d))
	}
	return out
}

// IsAdvisoryTriggered returns true if any tag in pageTags matches an active
// filter in cfg. Tag matching is case-insensitive and whitespace-trimmed.
func IsAdvisoryTriggered(pageTags []string, cfg FilterConfig) bool {
	// Normalise opt-out lists once, outside the per-tag loop.
	adultDis := normalizeTagList(cfg.AdultContent.Disabled)
	queerDis := normalizeTagList(cfg.QueerContent.Disabled)
	heavyDis := normalizeTagList(cfg.HeavyThemes.Disabled)
	substanceDis := normalizeTagList(cfg.SubstanceUse.Disabled)

	for _, tag := range pageTags {
		slug := strings.ToLower(strings.TrimSpace(tag))
		if cfg.AdultContent.Enabled && slices.Contains(AdultContentTags, slug) && !slices.Contains(adultDis, slug) {
			return true
		}
		if cfg.QueerContent.Enabled && slices.Contains(QueerContentTags, slug) && !slices.Contains(queerDis, slug) {
			return true
		}
		if cfg.HeavyThemes.Enabled && slices.Contains(HeavyThemesTags, slug) && !slices.Contains(heavyDis, slug) {
			return true
		}
		if cfg.SubstanceUse.Enabled && slices.Contains(SubstanceUseTags, slug) && !slices.Contains(substanceDis, slug) {
			return true
		}
	}
	return false
}
