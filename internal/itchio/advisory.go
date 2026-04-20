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
	Mature        bool           // single on/off, no per-tag opt-out
	LGBTQ         CategoryFilter
	HeavyThemes   CategoryFilter
	SubstanceUse  CategoryFilter
	SexualContent CategoryFilter
}

// MatureTags is the list of tag slugs considered explicit adult content.
// Users can toggle the whole category but cannot edit individual tags.
var MatureTags = []string{
	"adult", "boobs", "eroge", "erotic", "femdom", "gore",
	"hentai", "lewd", "nsfw", "nudity", "porn", "softcore",
	"tits", "titties", "xxx", "yaoi", "yuri",
}

// LGBTQTags is the list of tag slugs covering LGBTQ+ content and themes.
// Users can toggle the category and opt individual tags in or out.
// Sorted alphabetically.
var LGBTQTags = []string{
	"achillean", "aromantic", "asexual", "bisexual", "enby",
	"gay", "gender", "intersex", "lesbian", "lgbt", "lgbtq",
	"lgbtqia", "mlm", "non-binary", "nonbinary", "pansexual",
	"pride", "queer", "sapphic", "trans", "transgender", "wlw",
}

// HeavyThemesTags is the list of tag slugs covering potentially distressing
// narrative themes. Users can toggle the category and opt individual tags in
// or out. Sorted alphabetically.
var HeavyThemesTags = []string{
	"abuse", "anxiety", "bereavement", "child-loss", "death",
	"depression", "domestic-abuse", "eating-disorder", "grief",
	"loss", "mental-health", "mental-illness", "miscarriage",
	"self-harm", "sexual-assault", "suicide", "trauma", "war",
}

// SubstanceUseTags is the list of tag slugs covering drug and alcohol themes.
var SubstanceUseTags = []string{
	"addiction", "alcohol", "drug-use", "drugs", "substance-abuse",
}

// SexualContentTags is the list of tag slugs covering suggestive or
// non-explicit sexual content (distinct from the explicit MatureTags list).
var SexualContentTags = []string{
	"ecchi", "innuendo", "sexual-content", "sexy", "suggestive",
}

// IsAdvisoryTriggered returns true if any tag in pageTags matches an active
// filter in cfg. Tag matching is case-insensitive and whitespace-trimmed.
func IsAdvisoryTriggered(pageTags []string, cfg FilterConfig) bool {
	// Normalise opt-out lists once, outside the per-tag loop.
	norm := func(list []string) []string {
		out := make([]string, len(list))
		for i, d := range list {
			out[i] = strings.ToLower(strings.TrimSpace(d))
		}
		return out
	}
	lgbtqDis := norm(cfg.LGBTQ.Disabled)
	heavyDis := norm(cfg.HeavyThemes.Disabled)
	substanceDis := norm(cfg.SubstanceUse.Disabled)
	sexualDis := norm(cfg.SexualContent.Disabled)

	for _, tag := range pageTags {
		slug := strings.ToLower(strings.TrimSpace(tag))
		if cfg.Mature && slices.Contains(MatureTags, slug) {
			return true
		}
		if cfg.LGBTQ.Enabled && slices.Contains(LGBTQTags, slug) && !slices.Contains(lgbtqDis, slug) {
			return true
		}
		if cfg.HeavyThemes.Enabled && slices.Contains(HeavyThemesTags, slug) && !slices.Contains(heavyDis, slug) {
			return true
		}
		if cfg.SubstanceUse.Enabled && slices.Contains(SubstanceUseTags, slug) && !slices.Contains(substanceDis, slug) {
			return true
		}
		if cfg.SexualContent.Enabled && slices.Contains(SexualContentTags, slug) && !slices.Contains(sexualDis, slug) {
			return true
		}
	}
	return false
}
