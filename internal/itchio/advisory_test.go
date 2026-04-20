package itchio_test

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

func cfg(adult, queer, heavy, substance itchio.CategoryFilter) itchio.FilterConfig {
	return itchio.FilterConfig{
		AdultContent: adult,
		QueerContent: queer,
		HeavyThemes:  heavy,
		SubstanceUse: substance,
	}
}

var off = itchio.CategoryFilter{}

// ── Adult Content ─────────────────────────────────────────────────────────────

func TestAdultContentMatchExplicit(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if !itchio.IsAdvisoryTriggered([]string{"nsfw"}, cfg(on, off, off, off)) {
		t.Error("expected trigger for explicit adult tag 'nsfw'")
	}
}

func TestAdultContentMatchSuggestive(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if !itchio.IsAdvisoryTriggered([]string{"suggestive"}, cfg(on, off, off, off)) {
		t.Error("expected trigger for suggestive tag 'suggestive'")
	}
}

func TestAdultContentCaseInsensitive(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if !itchio.IsAdvisoryTriggered([]string{"NSFW"}, cfg(on, off, off, off)) {
		t.Error("expected trigger for uppercase 'NSFW'")
	}
}

func TestAdultContentDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"nsfw"}, cfg(off, off, off, off)) {
		t.Error("expected no trigger when adult content filter disabled")
	}
}

func TestAdultContentWhitespaceTrimmed(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if !itchio.IsAdvisoryTriggered([]string{" nsfw "}, cfg(on, off, off, off)) {
		t.Error("expected trigger for tag with surrounding whitespace")
	}
}

func TestAdultContentTagIndividuallyDisabled(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true, Disabled: []string{"nsfw"}}
	if itchio.IsAdvisoryTriggered([]string{"nsfw"}, cfg(on, off, off, off)) {
		t.Error("expected no trigger when nsfw individually disabled")
	}
}

func TestAdultContentOtherTagStillActive(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true, Disabled: []string{"nsfw"}}
	if !itchio.IsAdvisoryTriggered([]string{"porn"}, cfg(on, off, off, off)) {
		t.Error("expected trigger for 'porn' even when 'nsfw' individually disabled")
	}
}

// ── Queer Content ─────────────────────────────────────────────────────────────

func TestQueerContentMatch(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if !itchio.IsAdvisoryTriggered([]string{"queer"}, cfg(off, on, off, off)) {
		t.Error("expected trigger for queer content tag 'queer'")
	}
}

func TestQueerContentMasterDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"gay"}, cfg(off, off, off, off)) {
		t.Error("expected no trigger when queer content filter disabled")
	}
}

func TestQueerContentTagIndividuallyDisabled(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true, Disabled: []string{"lgbtq"}}
	if itchio.IsAdvisoryTriggered([]string{"lgbtq"}, cfg(off, on, off, off)) {
		t.Error("expected no trigger when lgbtq tag individually disabled")
	}
}

func TestQueerContentOtherTagStillActive(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true, Disabled: []string{"lgbtq"}}
	if !itchio.IsAdvisoryTriggered([]string{"gay"}, cfg(off, on, off, off)) {
		t.Error("expected trigger for 'gay' even when 'lgbtq' individually disabled")
	}
}

func TestQueerContentExpandedTags(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	for _, tag := range []string{"bisexual", "trans", "non-binary", "pansexual", "sapphic"} {
		if !itchio.IsAdvisoryTriggered([]string{tag}, cfg(off, on, off, off)) {
			t.Errorf("expected trigger for queer content tag %q", tag)
		}
	}
}

// ── Heavy Themes ──────────────────────────────────────────────────────────────

func TestHeavyThemesMatch(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if !itchio.IsAdvisoryTriggered([]string{"suicide"}, cfg(off, off, on, off)) {
		t.Error("expected trigger for heavy theme tag 'suicide'")
	}
}

func TestHeavyThemesDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"suicide"}, cfg(off, off, off, off)) {
		t.Error("expected no trigger when heavy themes filter disabled")
	}
}

func TestHeavyThemesTagIndividuallyDisabled(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true, Disabled: []string{"grief"}}
	if itchio.IsAdvisoryTriggered([]string{"grief"}, cfg(off, off, on, off)) {
		t.Error("expected no trigger when grief individually disabled")
	}
}

// ── Substance Use ─────────────────────────────────────────────────────────────

func TestSubstanceUseMatch(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if !itchio.IsAdvisoryTriggered([]string{"drugs"}, cfg(off, off, off, on)) {
		t.Error("expected trigger for substance use tag 'drugs'")
	}
}

func TestSubstanceUseDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"drugs"}, cfg(off, off, off, off)) {
		t.Error("expected no trigger when substance use filter disabled")
	}
}

// ── Cross-category ────────────────────────────────────────────────────────────

func TestAllFiltersOff(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"nsfw", "gay", "suicide", "drugs"},
		cfg(off, off, off, off)) {
		t.Error("expected no trigger when all filters disabled")
	}
}

func TestNonFlaggedTag(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if itchio.IsAdvisoryTriggered([]string{"platformer", "adventure"},
		cfg(on, on, on, on)) {
		t.Error("expected no trigger for non-flagged tags")
	}
}

func TestEmptyTags(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if itchio.IsAdvisoryTriggered(nil, cfg(on, on, on, on)) {
		t.Error("expected no trigger for nil tags")
	}
	if itchio.IsAdvisoryTriggered([]string{}, cfg(on, on, on, on)) {
		t.Error("expected no trigger for empty tags")
	}
}
