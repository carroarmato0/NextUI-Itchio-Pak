package itchio

// FeedPlatform describes one ROM platform and the itch.io RSS feed slugs that
// carry its games.
type FeedPlatform struct {
	Code      string   // NextUI system code, e.g. "GB"
	Name      string   // Human-readable label, e.g. "Game Boy"
	FeedSlugs []string // itch.io browse path segments, e.g. "made-with-gb-studio"
}

// AllPlatforms is the ordered catalogue of platforms fetched by FetchAllGames.
//
// Order matters for deduplication: a game that appears in multiple feeds is
// tagged with the platform of the first feed that contained it.  More specific
// platform feeds are listed before broader ones so that, for example, a GB
// Studio game that is also tagged "gameboy-color" on itch.io is classified as
// GBC rather than GB.
var AllPlatforms = []FeedPlatform{
	{
		Code:      "GBC",
		Name:      "Game Boy Color",
		FeedSlugs: []string{"tag-gameboy-color", "tag-gbc"},
	},
	{
		Code:      "GB",
		Name:      "Game Boy",
		FeedSlugs: []string{"made-with-gb-studio", "tag-gbstudio", "tag-gameboy-rom"},
	},
	{
		Code:      "GBA",
		Name:      "Game Boy Advance",
		FeedSlugs: []string{"tag-gba"},
	},
	{
		Code:      "NES",
		Name:      "Nintendo Entertainment System",
		FeedSlugs: []string{"tag-nes-rom"},
	},
	{
		Code:      "MD",
		Name:      "Sega Genesis",
		FeedSlugs: []string{"tag-sega-mega-drive", "tag-genesis-rom"},
	},
	{
		Code:      "P8",
		Name:      "Pico-8",
		FeedSlugs: []string{"tag-pico-8"},
	},
}
