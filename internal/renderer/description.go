package renderer

import (
	"strconv"
	"strings"
)

// A game description is parsed once into blocks and then only drawn. The
// parse used to run on every frame, and it lowercased the whole rest of the
// description for every paragraph — quadratic in the description's length,
// and about 7% of CPU and 19% of all allocations while a game page was open
// (profiled on a TrimUI Brick, 2026-09-26).

type descKind uint8

const (
	descText    descKind = iota // bare text outside any block tag
	descPara                    // <p>
	descHeading                 // <h2>
	descItem                    // <li>, with its bullet or number in prefix
	descListEnd                 // </ul> or </ol>
	descBreak                   // <br>
)

type descBlock struct {
	kind   descKind
	text   string
	prefix string // list items only
}

// parseDescription turns the limited HTML subset extractDescription produces
// (<p>, <br>, <h2>, <b>, <ul>, <ol>, <li>) into blocks. Inline tags inside a
// block are stripped. Empty blocks are dropped, as the drawing always did.
func parseDescription(markup string) []descBlock {
	var out []descBlock
	listType := "" // "ul" or "ol"
	listCounter := 0

	// closeBlock returns the block's content up to closing (searched without
	// regard to case) and advances i past it; an unclosed block runs to the end.
	i := 0
	closeBlock := func(closing string) string {
		idx := indexFoldASCII(markup[i:], closing)
		if idx < 0 {
			content := markup[i:]
			i = len(markup)
			return content
		}
		content := markup[i : i+idx]
		i += idx + len(closing)
		return content
	}

	for i < len(markup) {
		if markup[i] != '<' {
			end := strings.IndexByte(markup[i:], '<')
			var text string
			if end < 0 {
				text, i = strings.TrimSpace(markup[i:]), len(markup)
			} else {
				text, i = strings.TrimSpace(markup[i:i+end]), i+end
			}
			if text != "" {
				out = append(out, descBlock{kind: descText, text: text})
			}
			continue
		}

		end := strings.IndexByte(markup[i:], '>')
		if end < 0 {
			break
		}
		tag := asciiLower(strings.TrimSpace(markup[i+1 : i+end]))
		i += end + 1

		switch tag {
		case "p":
			if t := descStripInlineTags(closeBlock("</p>")); t != "" {
				out = append(out, descBlock{kind: descPara, text: t})
			}
		case "h2":
			if t := descStripInlineTags(strings.TrimSpace(closeBlock("</h2>"))); t != "" {
				out = append(out, descBlock{kind: descHeading, text: t})
			}
		case "ul":
			listType = "ul"
		case "ol":
			listType, listCounter = "ol", 0
		case "/ul", "/ol":
			listType, listCounter = "", 0
			out = append(out, descBlock{kind: descListEnd})
		case "li":
			if t := descStripInlineTags(strings.TrimSpace(closeBlock("</li>"))); t != "" {
				prefix := "•  "
				if listType == "ol" {
					listCounter++
					prefix = strconv.Itoa(listCounter) + ".  "
				}
				out = append(out, descBlock{kind: descItem, text: t, prefix: prefix})
			}
		case "br":
			out = append(out, descBlock{kind: descBreak})
		}
	}
	return out
}

// descStripInlineTags removes all HTML tags from s, returning plain text.
// Used by parseDescription to extract readable text from inline-tagged markup.
func descStripInlineTags(s string) string {
	var buf strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '<' {
			end := strings.IndexByte(s[i:], '>')
			if end < 0 {
				break
			}
			i += end + 1
			continue
		}
		buf.WriteByte(s[i])
		i++
	}
	return strings.Join(strings.Fields(buf.String()), " ")
}

// indexFoldASCII is strings.Index ignoring ASCII case, without building a
// lowercased copy — which for some non-ASCII text would also have a different
// byte length, so its offsets would not fit the original.
func indexFoldASCII(s, sub string) int {
	n := len(sub)
	for i := 0; i+n <= len(s); i++ {
		match := true
		for j := 0; j < n; j++ {
			if lowerASCII(s[i+j]) != lowerASCII(sub[j]) {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func lowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// asciiLower lowercases the ASCII letters of a short string (a tag name).
func asciiLower(s string) string {
	for i := 0; i < len(s); i++ {
		if 'A' <= s[i] && s[i] <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				b[j] = lowerASCII(b[j])
			}
			return string(b)
		}
	}
	return s
}
