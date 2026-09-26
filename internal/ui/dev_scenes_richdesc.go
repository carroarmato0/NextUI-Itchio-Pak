//go:build !headless

package ui

// A game page whose description uses every tag DrawFormattedText handles —
// paragraphs, headings, both list kinds, breaks, inline bold, mixed case —
// so the palette audit and layout checks cover formatted text, not just the
// plain fixture.
func init() {
	devScenes = append(devScenes, Scene{Name: "detail-rich-description",
		Desc: "Game detail whose description uses every supported tag",
		Build: func(d SceneDeps) Screen {
			s := devDetail(d)
			det := *s.detail
			det.Description = `An opening line outside any block.` +
				`<p>Tobu is back, and the sky is <b>higher than ever</b>. Bounce on birds, ` +
				`clouds and the occasional falling piano to reach the top.</p>` +
				`<H2>Features</H2>` +
				`<ul><li>Fifty hand-made levels</li><li>A <b>hard mode</b> for the brave</li>` +
				`<LI>Save states on any handheld</LI></ul>` +
				`<h2>How to play</h2>` +
				`<ol><li>Press A to jump</li><li>Hold B to dash through clouds</li><li>Never look down</li></ol>` +
				`<P>Made for the Game Boy Color.<br>Also runs on the original Game Boy.</P>` +
				`Trailing text after the last block.`
			s.detail = &det
			return s
		}})
}
