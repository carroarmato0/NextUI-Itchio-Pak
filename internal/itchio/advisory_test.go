package itchio_test

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

func cfg(mature bool, lgbtq, heavy, substance, sexual itchio.CategoryFilter) itchio.FilterConfig {
	return itchio.FilterConfig{
		Mature:        mature,
		LGBTQ:         lgbtq,
		HeavyThemes:   heavy,
		SubstanceUse:  substance,
		SexualContent: sexual,
	}
}

var offAll = itchio.CategoryFilter{}

// ── Mature ────────────────────────────────────────────────────────────────────

func TestMatureMatch(t *testing.T) {
	if !itchio.IsAdvisoryTriggered([]string{"nsfw"}, cfg(true, offAll, offAll, offAll, offAll)) {
		t.Error("expected trigger for mature tag 'nsfw'")
	}
}

func TestMatureCaseInsensitive(t *testing.T) {
	if !itchio.IsAdvisoryTriggered([]string{"NSFW"}, cfg(true, offAll, offAll, offAll, offAll)) {
		t.Error("expected trigger for uppercase 'NSFW'")
	}
}

func TestMatureDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"nsfw"}, cfg(false, offAll, offAll, offAll, offAll)) {
		t.Error("expected no trigger when mature filter disabled")
	}
}

func TestMatureWhitespaceTrimmed(t *testing.T) {
	if !itchio.IsAdvisoryTriggered([]string{" nsfw "}, cfg(true, offAll, offAll, offAll, offAll)) {
		t.Error("expected trigger for tag with surrounding whitespace")
	}
}

// ── LGBTQ ─────────────────────────────────────────────────────────────────────

func TestLGBTQMatch(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if !itchio.IsAdvisoryTriggered([]string{"lgbtq"}, cfg(false, on, offAll, offAll, offAll)) {
		t.Error("expected trigger for lgbtq tag")
	}
}

func TestLGBTQMasterDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"lgbtq"}, cfg(false, offAll, offAll, offAll, offAll)) {
		t.Error("expected no trigger when lgbtq filter disabled")
	}
}

func TestLGBTQTagIndividuallyDisabled(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true, Disabled: []string{"lgbtq"}}
	if itchio.IsAdvisoryTriggered([]string{"lgbtq"}, cfg(false, on, offAll, offAll, offAll)) {
		t.Error("expected no trigger when lgbtq tag individually disabled")
	}
}

func TestLGBTQOtherTagStillActive(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true, Disabled: []string{"lgbtq"}}
	if !itchio.IsAdvisoryTriggered([]string{"gay"}, cfg(false, on, offAll, offAll, offAll)) {
		t.Error("expected trigger for 'gay' even when 'lgbtq' individually disabled")
	}
}

func TestLGBTQExpandedTags(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	for _, tag := range []string{"queer", "bisexual", "trans", "non-binary", "pansexual"} {
		if !itchio.IsAdvisoryTriggered([]string{tag}, cfg(false, on, offAll, offAll, offAll)) {
			t.Errorf("expected trigger for expanded lgbtq tag %q", tag)
		}
	}
}

// ── Heavy Themes ──────────────────────────────────────────────────────────────

func TestHeavyThemesMatch(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if !itchio.IsAdvisoryTriggered([]string{"suicide"}, cfg(false, offAll, on, offAll, offAll)) {
		t.Error("expected trigger for heavy theme tag 'suicide'")
	}
}

func TestHeavyThemesDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"suicide"}, cfg(false, offAll, offAll, offAll, offAll)) {
		t.Error("expected no trigger when heavy themes filter disabled")
	}
}

func TestHeavyThemesTagIndividuallyDisabled(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true, Disabled: []string{"grief"}}
	if itchio.IsAdvisoryTriggered([]string{"grief"}, cfg(false, offAll, on, offAll, offAll)) {
		t.Error("expected no trigger when grief individually disabled")
	}
}

// ── Substance Use ─────────────────────────────────────────────────────────────

func TestSubstanceUseMatch(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if !itchio.IsAdvisoryTriggered([]string{"drugs"}, cfg(false, offAll, offAll, on, offAll)) {
		t.Error("expected trigger for substance use tag 'drugs'")
	}
}

func TestSubstanceUseDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"drugs"}, cfg(false, offAll, offAll, offAll, offAll)) {
		t.Error("expected no trigger when substance use filter disabled")
	}
}

// ── Sexual Content ────────────────────────────────────────────────────────────

func TestSexualContentMatch(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if !itchio.IsAdvisoryTriggered([]string{"suggestive"}, cfg(false, offAll, offAll, offAll, on)) {
		t.Error("expected trigger for sexual content tag 'suggestive'")
	}
}

func TestSexualContentDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"suggestive"}, cfg(false, offAll, offAll, offAll, offAll)) {
		t.Error("expected no trigger when sexual content filter disabled")
	}
}

// ── Cross-category ────────────────────────────────────────────────────────────

func TestAllFiltersOff(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"nsfw", "lgbtq", "suicide", "drugs", "suggestive"},
		cfg(false, offAll, offAll, offAll, offAll)) {
		t.Error("expected no trigger when all filters disabled")
	}
}

func TestNonFlaggedTag(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if itchio.IsAdvisoryTriggered([]string{"platformer", "adventure"},
		cfg(true, on, on, on, on)) {
		t.Error("expected no trigger for non-flagged tags")
	}
}

func TestEmptyTags(t *testing.T) {
	on := itchio.CategoryFilter{Enabled: true}
	if itchio.IsAdvisoryTriggered(nil, cfg(true, on, on, on, on)) {
		t.Error("expected no trigger for nil tags")
	}
	if itchio.IsAdvisoryTriggered([]string{}, cfg(true, on, on, on, on)) {
		t.Error("expected no trigger for empty tags")
	}
}
