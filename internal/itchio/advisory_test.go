package itchio_test

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

func TestIsAdvisoryTriggered_EmptyTags(t *testing.T) {
	if itchio.IsAdvisoryTriggered(nil, true, true, nil) {
		t.Error("expected no trigger for nil tags")
	}
	if itchio.IsAdvisoryTriggered([]string{}, true, true, nil) {
		t.Error("expected no trigger for empty tags")
	}
}

func TestIsAdvisoryTriggered_MatureMatch(t *testing.T) {
	if !itchio.IsAdvisoryTriggered([]string{"nsfw"}, true, false, nil) {
		t.Error("expected trigger for mature tag 'nsfw'")
	}
}

func TestIsAdvisoryTriggered_MatureCaseInsensitive(t *testing.T) {
	if !itchio.IsAdvisoryTriggered([]string{"NSFW"}, true, false, nil) {
		t.Error("expected trigger for uppercase 'NSFW'")
	}
}

func TestIsAdvisoryTriggered_MatureDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"nsfw"}, false, false, nil) {
		t.Error("expected no trigger when mature filter disabled")
	}
}

func TestIsAdvisoryTriggered_SensitiveMatch(t *testing.T) {
	if !itchio.IsAdvisoryTriggered([]string{"lgbtq"}, false, true, nil) {
		t.Error("expected trigger for sensitive tag 'lgbtq'")
	}
}

func TestIsAdvisoryTriggered_SensitiveDisabledMaster(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"lgbtq"}, false, false, nil) {
		t.Error("expected no trigger when sensitive filter disabled")
	}
}

func TestIsAdvisoryTriggered_SensitiveTagIndividuallyDisabled(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"lgbtq"}, false, true, []string{"lgbtq"}) {
		t.Error("expected no trigger when tag is in SensitiveDisabled")
	}
}

func TestIsAdvisoryTriggered_SensitiveOtherTagsStillActive(t *testing.T) {
	// "gay" is not in SensitiveDisabled, so it should still trigger
	if !itchio.IsAdvisoryTriggered([]string{"gay"}, false, true, []string{"lgbtq"}) {
		t.Error("expected trigger for 'gay' even when 'lgbtq' is individually disabled")
	}
}

func TestIsAdvisoryTriggered_BothFiltersOff(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"nsfw", "lgbtq"}, false, false, nil) {
		t.Error("expected no trigger when both filters disabled")
	}
}

func TestIsAdvisoryTriggered_NonFlaggedTag(t *testing.T) {
	if itchio.IsAdvisoryTriggered([]string{"platformer", "adventure"}, true, true, nil) {
		t.Error("expected no trigger for non-flagged tags")
	}
}
