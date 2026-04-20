package itchio

import "strings"

// MatureTags is the hardcoded list of tag slugs considered mature content.
// Parents can enable/disable the whole category but cannot edit this list.
var MatureTags = []string{
	"adult", "boobs", "eroge", "erotic", "femdom", "gore",
	"hentai", "lewd", "nsfw", "nudity", "porn", "softcore",
	"tits", "titties", "xxx", "yaoi", "yuri",
}

// SensitiveTags is the hardcoded list of tag slugs considered sensitive topics.
// Parents can enable/disable the whole category and toggle individual tags.
// Sorted alphabetically.
var SensitiveTags = []string{
	"gay", "gender", "lesbian", "lgbtq", "sexy", "transgender",
}

// IsAdvisoryTriggered returns true if any tag in pageTags should trigger the
// parental advisory overlay. It takes the filter configuration as plain values
// so it does not depend on the settings package.
//
//   - matureEnabled:    whether the Mature Content filter is active
//   - sensitiveEnabled: whether the Sensitive Topics filter is active
//   - sensitiveDisabled: individual sensitive tags that are turned off
func IsAdvisoryTriggered(pageTags []string, matureEnabled, sensitiveEnabled bool, sensitiveDisabled []string) bool {
	for _, tag := range pageTags {
		slug := strings.ToLower(strings.TrimSpace(tag))
		if matureEnabled && containsStr(MatureTags, slug) {
			return true
		}
		if sensitiveEnabled && containsStr(SensitiveTags, slug) && !containsStr(sensitiveDisabled, slug) {
			return true
		}
	}
	return false
}

// containsStr reports whether list contains s.
func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
