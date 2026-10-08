package renderer

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseDescription(t *testing.T) {
	markup := `Intro text <P>First  <b>para</b>
graph.</P><H2>Features</h2><ul><li>One</li><li> Two <i>x</i></LI></ul>` +
		`<ol><li>A</li><li>B</li></ol><br><p></p><h2>  </h2>tail`
	got := parseDescription(markup)
	want := []descBlock{
		{kind: descText, text: "Intro text"},
		{kind: descPara, text: "First para graph."},
		{kind: descHeading, text: "Features"},
		{kind: descItem, prefix: "•  ", text: "One"},
		{kind: descItem, prefix: "•  ", text: "Two x"},
		{kind: descListEnd},
		{kind: descItem, prefix: "1.  ", text: "A"},
		{kind: descItem, prefix: "2.  ", text: "B"},
		{kind: descListEnd},
		{kind: descBreak},
		{kind: descText, text: "tail"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%#v\nwant\n%#v", got, want)
	}
}

// Unclosed tags run to the end of the markup, as before.
func TestParseDescription_unclosed(t *testing.T) {
	got := parseDescription(`<p>no end <b>bold`)
	if len(got) != 1 || got[0].kind != descPara || got[0].text != "no end bold" {
		t.Errorf("got %#v", got)
	}
	if got := parseDescription(`text <p`); len(got) != 1 || got[0].text != "text" {
		t.Errorf("a tag without '>' should stop parsing: %#v", got)
	}
}

// The old parser lowercased the whole rest of the markup for every block, on
// every frame — quadratic in the description's length. Parsing a long
// description must stay roughly linear.
func TestParseDescription_linear(t *testing.T) {
	para := "<p>" + strings.Repeat("word ", 40) + "</p>"
	small, big := strings.Repeat(para, 50), strings.Repeat(para, 400)
	a := testing.AllocsPerRun(5, func() { parseDescription(small) })
	b := testing.AllocsPerRun(5, func() { parseDescription(big) })
	if b > a*10 { // 8x the input; quadratic would be ~64x
		t.Errorf("allocations grew %0.f -> %0.f for 8x the input", a, b)
	}
}

// Case-insensitive search must not shift offsets for non-ASCII text, which
// strings.ToLower could (it changes the byte length of some characters).
func TestIndexFoldASCII(t *testing.T) {
	s := "İstanbul – ÇAY </P> rest"
	if i := indexFoldASCII(s, "</p>"); s[i:i+4] != "</P>" {
		t.Errorf("index %d points at %q", i, s[i:i+4])
	}
	if indexFoldASCII("abc", "</p>") != -1 {
		t.Error("found a tag that is not there")
	}
}

// BenchmarkParseDescription_real parses a real description (Tobu Tobu Girl
// Deluxe's page, as extractDescription returns it). The game page used to do
// this on every frame, with an extra full lowercase copy of the remaining
// markup per block; it now happens once per page.
func BenchmarkParseDescription_real(b *testing.B) {
	markup := benchDescription
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parseDescription(markup)
	}
}

// benchDescription is a long, list-heavy description of the shape itch.io pages have.
var benchDescription = strings.Repeat(`<p>Tobu is back, and the sky is <b>higher than ever</b>. Bounce on birds, clouds and the occasional falling piano.</p>`+
	`<h2>Features</h2><ul><li>Fifty hand-made levels</li><li>A hard mode for the brave</li><li>Save states</li></ul>`, 12)
