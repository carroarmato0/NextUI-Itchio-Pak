# Detail Screen Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the game detail screen to show all key content above the fold (screenshot strip + QR + action/status + tags + description start), and render the description with basic HTML formatting (paragraphs, headings, bullet lists).

**Architecture:** Three independent layers — (1) `extractDescription` in `game.go` now outputs a markup subset instead of plain text; (2) a new `DrawFormattedText` method in `renderer.go` parses that markup into block-level rendering calls; (3) `screen_detail.go` gets a new layout with smaller screenshot, wider QR column, merged action+status row, and calls `DrawFormattedText`. Each layer is independently testable/verifiable.

**Tech Stack:** Go 1.22+, SDL2 via go-sdl2, `golang.org/x/net/html` (already imported for HTML parsing). All SDL2 screens use `//go:build !headless`; logic layers are headless-testable.

---

### Task 1: Update `extractDescription` to output markup

**Files:**
- Modify: `internal/itchio/game.go` (replace `extractText` closure in `extractDescription`)
- Modify: `internal/itchio/game_test.go` (add new tests, update existing ones)

- [ ] **Step 1: Write failing tests**

Add to `internal/itchio/game_test.go`:

```go
func TestExtractDescriptionPreservesStructuralMarkup(t *testing.T) {
	const pageHTML = `<html><body>
<div class="formatted_description user_formatted">
<h2>Features</h2>
<ul>
  <li>Multiple episodes</li>
  <li>Save states</li>
</ul>
<p>A paragraph with <strong>bold</strong> and <em>italic</em> text.</p>
<p>Second paragraph.</p>
</div>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(pageHTML))
	}))
	defer srv.Close()

	c := itchio.NewClient()
	detail, err := c.FetchGameDetail(srv.URL)
	if err != nil {
		t.Fatalf("FetchGameDetail: %v", err)
	}
	desc := detail.Description

	// Block-level tags must be preserved
	if !strings.Contains(desc, "<h2>") {
		t.Errorf("expected <h2> tag, got: %q", desc)
	}
	if !strings.Contains(desc, "<ul>") {
		t.Errorf("expected <ul> tag, got: %q", desc)
	}
	if !strings.Contains(desc, "<li>") {
		t.Errorf("expected <li> tag, got: %q", desc)
	}
	if !strings.Contains(desc, "<p>") {
		t.Errorf("expected <p> tag, got: %q", desc)
	}

	// Inline tags: strong/em both map to <b>
	if !strings.Contains(desc, "<b>") {
		t.Errorf("expected <b> tag (from strong/em), got: %q", desc)
	}

	// Text content preserved
	if !strings.Contains(desc, "Features") {
		t.Errorf("missing heading text in: %q", desc)
	}
	if !strings.Contains(desc, "Multiple episodes") {
		t.Errorf("missing list item text in: %q", desc)
	}
	if !strings.Contains(desc, "bold") {
		t.Errorf("missing bold text in: %q", desc)
	}

	// Disallowed tags stripped
	if strings.Contains(desc, "<a") || strings.Contains(desc, "<div") || strings.Contains(desc, "<span") {
		t.Errorf("disallowed tags present in: %q", desc)
	}
}

func TestExtractDescriptionOrderedList(t *testing.T) {
	const pageHTML = `<html><body>
<div class="formatted_description">
<ol><li>First</li><li>Second</li></ol>
</div></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(pageHTML))
	}))
	defer srv.Close()

	c := itchio.NewClient()
	detail, _ := c.FetchGameDetail(srv.URL)
	desc := detail.Description
	if !strings.Contains(desc, "<ol>") {
		t.Errorf("expected <ol> tag, got: %q", desc)
	}
	if !strings.Contains(desc, "First") || !strings.Contains(desc, "Second") {
		t.Errorf("missing list text in: %q", desc)
	}
}

func TestExtractDescriptionStripsButtonsAndEmbeds(t *testing.T) {
	const pageHTML = `<html><body>
<div class="formatted_description">
<button>Click me</button>
<iframe src="https://youtube.com/embed/xxx"></iframe>
<p>Real content here.</p>
</div></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(pageHTML))
	}))
	defer srv.Close()

	c := itchio.NewClient()
	detail, _ := c.FetchGameDetail(srv.URL)
	desc := detail.Description
	if strings.Contains(desc, "Click me") {
		t.Errorf("button text should be stripped, got: %q", desc)
	}
	if !strings.Contains(desc, "Real content") {
		t.Errorf("missing paragraph content in: %q", desc)
	}
}
```

- [ ] **Step 2: Run tests — expect failures**

```bash
./scripts/test.sh 2>&1 | grep -E "FAIL|extractDescription|Preserves|Ordered|Strips"
```

Expected: all three new tests fail (current `extractDescription` returns plain text with no markup tags).

- [ ] **Step 3: Replace `extractDescription` in `internal/itchio/game.go`**

Find and replace the entire `extractDescription` function (lines ~150–222) with:

```go
// extractDescription pulls the game description from the page HTML and
// returns a lightweight markup string preserving paragraph, heading, and
// list structure while stripping scripts, embeds, links, and unknown tags.
// Inline <strong>/<b>/<em>/<i> are normalised to <b>; headings to <h2>.
func extractDescription(pageHTML string) string {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return ""
	}

	// Find the first div whose class contains "formatted_description".
	var descNode *html.Node
	var findDiv func(*html.Node)
	findDiv = func(n *html.Node) {
		if descNode != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "div" {
			for _, a := range n.Attr {
				if a.Key == "class" && strings.Contains(a.Val, "formatted_description") {
					descNode = n
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findDiv(c)
		}
	}
	findDiv(doc)
	if descNode == nil {
		return ""
	}

	// Convert the subtree to lightweight markup, preserving block structure.
	var buf bytes.Buffer
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "button", "iframe", "video", "audio", "script", "style", "a":
				return // strip entirely
			case "br":
				buf.WriteString("<br>")
				return
			case "p":
				buf.WriteString("<p>")
				for c := n.FirstChild; c != nil; c = c.NextSibling { walk(c) }
				buf.WriteString("</p>")
				return
			case "h1", "h2", "h3", "h4", "h5", "h6":
				buf.WriteString("<h2>")
				for c := n.FirstChild; c != nil; c = c.NextSibling { walk(c) }
				buf.WriteString("</h2>")
				return
			case "strong", "b", "em", "i":
				buf.WriteString("<b>")
				for c := n.FirstChild; c != nil; c = c.NextSibling { walk(c) }
				buf.WriteString("</b>")
				return
			case "ul":
				buf.WriteString("<ul>")
				for c := n.FirstChild; c != nil; c = c.NextSibling { walk(c) }
				buf.WriteString("</ul>")
				return
			case "ol":
				buf.WriteString("<ol>")
				for c := n.FirstChild; c != nil; c = c.NextSibling { walk(c) }
				buf.WriteString("</ol>")
				return
			case "li":
				buf.WriteString("<li>")
				for c := n.FirstChild; c != nil; c = c.NextSibling { walk(c) }
				buf.WriteString("</li>")
				return
			case "tr":
				buf.WriteString("<br>")
			case "td", "th":
				if buf.Len() > 0 {
					buf.WriteString(" ")
				}
			}
		}
		if n.Type == html.TextNode {
			buf.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(descNode)

	return strings.TrimSpace(buf.String())
}
```

Also remove the `htmlToPlainText` function (lines ~224–269) — it is no longer used after this change.

- [ ] **Step 4: Run tests — expect all pass**

```bash
./scripts/test.sh 2>&1 | grep -E "ok|FAIL"
```

Expected: all packages `ok`, including `internal/itchio`.

- [ ] **Step 5: Commit**

```bash
git add internal/itchio/game.go internal/itchio/game_test.go
git commit -m "feat(itchio): extractDescription preserves structural HTML markup

Returns lightweight markup (<p>,<h2>,<b>,<ul>,<ol>,<li>,<br>) instead of
plain text. Inline strong/em normalised to <b>; headings to <h2>.
Links, buttons, embeds stripped as before. Enables DrawFormattedText to
render paragraph spacing, headings, and bullet lists in the detail screen."
```

---

### Task 2: Add `DrawFormattedText` to renderer

**Files:**
- Modify: `internal/renderer/renderer.go` (add two new exported/unexported functions after `DrawWrappedText`)

No automated test — SDL2 only. Verified by build + device screenshot in Task 7.

- [ ] **Step 1: Add helper `descStripInlineTags` before the new method**

Add immediately after `DrawWrappedText` (around line 529 in renderer.go):

```go
// descStripInlineTags removes all HTML tags from s, returning plain text.
// Used by DrawFormattedText to extract readable text from inline-tagged markup.
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

// descClampU8 clamps an int value to [0, 255].
func descClampU8(v int) uint8 {
	if v > 255 {
		return 255
	}
	if v < 0 {
		return 0
	}
	return uint8(v)
}
```

- [ ] **Step 2: Add `DrawFormattedText` method**

Add immediately after the helpers above:

```go
// DrawFormattedText renders a description string containing a limited subset
// of HTML markup produced by extractDescription: <p>, <br>, <h2>, <b>,
// <ul>, <ol>, <li>. Block-level structure (paragraphs, headings, list items)
// is honoured; inline <b> content is rendered in a slightly brighter colour.
// Returns the total pixel height consumed.
func (r *Renderer) DrawFormattedText(markup string, x, y, maxW, lineH int32,
	baseR, baseG, baseB uint8) int32 {
	startY := y
	_, fontH := r.TextSize("Ag")

	// Brightened colour for headings and bold text.
	boldR := descClampU8(int(baseR) + 55)
	boldG := descClampU8(int(baseG) + 55)
	boldB := descClampU8(int(baseB) + 55)

	listType := "" // "ul" or "ol"
	listCounter := 0

	lower := strings.ToLower

	i := 0
	for i < len(markup) {
		if markup[i] != '<' {
			// Bare text — find next tag
			end := strings.IndexByte(markup[i:], '<')
			var text string
			if end < 0 {
				text = strings.TrimSpace(markup[i:])
				i = len(markup)
			} else {
				text = strings.TrimSpace(markup[i : i+end])
				i += end
			}
			if text != "" {
				y += r.DrawWrappedText(text, x, y, maxW, lineH, baseR, baseG, baseB)
			}
			continue
		}

		// Parse tag
		end := strings.IndexByte(markup[i:], '>')
		if end < 0 {
			break
		}
		tag := strings.TrimSpace(lower(markup[i+1 : i+end]))
		i += end + 1

		switch tag {
		case "p":
			closeIdx := strings.Index(lower(markup[i:]), "</p>")
			if closeIdx < 0 {
				closeIdx = len(markup) - i
			}
			pText := descStripInlineTags(markup[i : i+closeIdx])
			if pText != "" {
				if y > startY {
					y += fontH / 3
				}
				y += r.DrawWrappedText(pText, x, y, maxW, lineH, baseR, baseG, baseB)
				y += fontH / 3
			}
			if closeIdx < len(markup)-i {
				i += closeIdx + 4
			} else {
				i = len(markup)
			}

		case "h2":
			closeIdx := strings.Index(lower(markup[i:]), "</h2>")
			if closeIdx < 0 {
				closeIdx = len(markup) - i
			}
			hText := descStripInlineTags(strings.TrimSpace(markup[i : i+closeIdx]))
			if hText != "" {
				if y > startY {
					y += fontH / 2
				}
				r.DrawBoldText(hText, x, y, boldR, boldG, boldB)
				y += lineH + fontH/4
			}
			if closeIdx < len(markup)-i {
				i += closeIdx + 5
			} else {
				i = len(markup)
			}

		case "ul":
			listType = "ul"
		case "ol":
			listType = "ol"
			listCounter = 0
		case "/ul", "/ol":
			listType = ""
			listCounter = 0
			y += fontH / 4

		case "li":
			closeIdx := strings.Index(lower(markup[i:]), "</li>")
			if closeIdx < 0 {
				closeIdx = len(markup) - i
			}
			liText := descStripInlineTags(strings.TrimSpace(markup[i : i+closeIdx]))
			if liText != "" {
				var prefix string
				if listType == "ol" {
					listCounter++
					prefix = fmt.Sprintf("%d.  ", listCounter)
				} else {
					prefix = "•  "
				}
				_, smallFH := r.SmallTextSize("Ag")
				pw, _ := r.SmallTextSize(prefix)
				r.DrawSmallText(prefix, x, y+(lineH-smallFH)/2, baseR, baseG, baseB)
				y += r.DrawWrappedText(liText, x+pw, y, maxW-pw, lineH, baseR, baseG, baseB)
				y += fontH / 6
			}
			if closeIdx < len(markup)-i {
				i += closeIdx + 5
			} else {
				i = len(markup)
			}

		case "br":
			y += lineH / 2
		}
	}
	return y - startY
}
```

- [ ] **Step 3: Add `"fmt"` and `"strings"` imports if not already present**

Both `fmt` and `strings` are already imported in `renderer.go`. Verify:

```bash
grep '"fmt"\|"strings"' internal/renderer/renderer.go | head -3
```

- [ ] **Step 4: Build to verify compile**

```bash
./scripts/build.sh native 2>&1 | tail -3
```

Expected: `Built: bin/native/itchio-pak ...`

- [ ] **Step 5: Commit**

```bash
git add internal/renderer/renderer.go
git commit -m "feat(renderer): add DrawFormattedText for HTML-structured descriptions

Parses lightweight markup (<p>,<h2>,<b>,<ul>,<ol>,<li>,<br>) and renders
with block-level formatting: paragraph gaps, bold headings (BoldText),
bullet/numbered list items with indent. Inline <b> rendered in brighter
colour. Returns total height consumed."
```

---

### Task 3: Detail screen — header badges (platform + download status)

**Files:**
- Modify: `internal/ui/screen_detail.go` (header drawing section, ~lines 265–278)

No automated test — SDL2. Verified in Task 7.

- [ ] **Step 1: Locate the header drawing block**

In `screen_detail.go` find the section that draws title + author (around line 272):

```go
mt := r.Theme.MainText
title := truncateToWidth(r, s.game.Title, r.W-24)
blockH := mainFH + 4 + smallFH
titleY := (headerH - blockH) / 2
r.DrawText(title, 12, titleY, mt[0], mt[1], mt[2])
ht := r.Theme.HintText
r.DrawSmallText("by "+s.game.Author, 12, titleY+mainFH+4, ht[0], ht[1], ht[2])
```

- [ ] **Step 2: Add platform and downloaded status badges**

Replace the block above with:

```go
mt := r.Theme.MainText
ht := r.Theme.HintText

// Right-side badges — platform and download status.
// Compute badge positions right-to-left so they never overlap with title.
badgeRightEdge := r.W - 10
badgePillH := smallFH + 4

if s.game.Platform != "" {
    platLabel := s.game.Platform
    pw, _ := r.SmallTextSize(platLabel)
    platPillW := pw + 12
    platPillX := badgeRightEdge - platPillW
    platPillY := (headerH - badgePillH) / 2
    r.DrawPill(platPillX, platPillY, platPillW, badgePillH, 35, 45, 65)
    r.DrawSmallTextCenteredInRect(platLabel, platPillX, platPillY, platPillW, badgePillH, 130, 170, 210)
    badgeRightEdge = platPillX - 6
}

// Note: "● Owned" badge (for games owned via API key but not yet downloaded)
// is deferred — DetailScreen does not carry the ownedURLs map.
if s.detail != nil && s.inv.IsPresent(s.game.URL) {
    dlLabel := "● Downloaded"
    if r.W <= narrowScreenW {
        dlLabel = "● DL"
    }
    dw, _ := r.SmallTextSize(dlLabel)
    dlPillW := dw + 12
    dlPillX := badgeRightEdge - dlPillW
    dlPillY := (headerH - badgePillH) / 2
    r.DrawPill(dlPillX, dlPillY, dlPillW, badgePillH, 25, 50, 30)
    r.DrawSmallTextCenteredInRect(dlLabel, dlPillX, dlPillY, dlPillW, badgePillH, 70, 190, 90)
    badgeRightEdge = dlPillX - 6
}

// Title truncated to stay clear of badges.
maxTitleW := badgeRightEdge - 16
title := truncateToWidth(r, s.game.Title, maxTitleW)
blockH := mainFH + 4 + smallFH
titleY := (headerH - blockH) / 2
r.DrawText(title, 12, titleY, mt[0], mt[1], mt[2])
r.DrawSmallText("by "+s.game.Author, 12, titleY+mainFH+4, ht[0], ht[1], ht[2])
```

The `ht` variable is now declared earlier; remove the duplicate declaration that follows in the original code if present.

- [ ] **Step 3: Fix the header redraw at the bottom of Draw()**

The same header is redrawn after ClearClipRect (around line 547) to prevent content scrolling over it. Apply the same changes there — find the `r.DrawText(title, 12, titleY, ...)` + `r.DrawSmallText("by "...)` block and replace it with the same code from Step 2.

- [ ] **Step 4: Build**

```bash
./scripts/build.sh native 2>&1 | tail -3
```

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screen_detail.go
git commit -m "feat(detail): add platform and download-status badges to header"
```

---

### Task 4: Detail screen — Row 1: smaller screenshot + narrower QR

**Files:**
- Modify: `internal/ui/screen_detail.go` (geometry variables, ~lines 290–293)

- [ ] **Step 1: Find and update the shared geometry block**

In `screen_detail.go` find (around line 290):

```go
qrColW := r.W / 4
imgAreaW := r.W - qrColW - margin - 10
imgBoxW := imgAreaW - margin
imgBoxH := contentH * 2 / 3
```

Replace with:

```go
// QR column narrower than before — screenshot gets more horizontal space.
qrColW := r.W / 5
if r.W <= narrowScreenW {
    qrColW = r.W / 6
}
imgAreaW := r.W - qrColW - margin - 10
imgBoxW := imgAreaW - margin
// Screenshot height: ~40% of content area. Cap so it doesn't exceed
// a 16:9 ratio on very wide screens (avoids tall letterboxed images).
imgBoxH := contentH * 40 / 100
maxByWidth := imgBoxW * 9 / 16
if imgBoxH > maxByWidth {
    imgBoxH = maxByWidth
}
```

- [ ] **Step 2: Build**

```bash
./scripts/build.sh native 2>&1 | tail -3
```

- [ ] **Step 3: Commit**

```bash
git add internal/ui/screen_detail.go
git commit -m "feat(detail): reduce screenshot to 40% height, narrow QR column"
```

---

### Task 5: Detail screen — Row 2: action button + status card on one line

**Files:**
- Modify: `internal/ui/screen_detail.go` (action + status card section, ~lines 388–511)

This is the most surgical change. The goal: when the game is downloaded, draw the action button and the status card side-by-side on the **same** row instead of stacking them.

- [ ] **Step 1: Locate the action section start**

Find the `// ── Action area (full width)` comment (~line 388) and the `isPresent := s.inv.IsPresent(s.game.URL)` line.

- [ ] **Step 2: Replace the downloaded-game action block**

Find the `if isPresent {` block. Its first call is `drawActionRow("A", "Download again", ...)` or similar. Replace the entire `if isPresent {` block with:

```go
if isPresent {
    // Row 2: action button LEFT + status card RIGHT on the same line.
    rowY := y
    rowH := fontH + 14
    _, smallFH := r.SmallTextSize("Ag")
    aT := r.Theme.AccentText

    // Determine action label.
    var actionLabel string
    var actionR, actionG, actionB uint8
    if s.game.IsFree {
        actionLabel, actionR, actionG, actionB = "Download again", 80, 200, 80
    } else if s.cfg.APIKey == "" {
        actionLabel, actionR, actionG, actionB = "Purchase required", 220, 180, 60
    } else {
        actionLabel, actionR, actionG, actionB = "Download again", 80, 200, 80
    }

    // Draw action badge + label.
    d := fontH + 4
    r.DrawCircleBadge(margin+d/2, rowY+d/2, d, ac[0], ac[1], ac[2])
    r.DrawSmallTextCenteredInRect("A", margin, rowY, d, d, aT[0], aT[1], aT[2])
    r.DrawText(actionLabel, margin+d+8, rowY, actionR, actionG, actionB)
    alW, _ := r.TextSize(actionLabel)
    actionEndX := margin + d + 8 + alW

    // Status card occupies the remaining width on the same row.
    if entry, ok := s.inv.Lookup(s.game.URL); ok && len(entry.Files) > 0 {
        f := entry.Files[0]
        pathText := f.Filename + " → " + filepath.Dir(f.DestPath) + "/"
        if len(entry.Files) > 1 {
            pathText += " (+" + strconv.Itoa(len(entry.Files)-1) + " more)"
        }

        const cp = int32(8)
        const dlBp = int32(5)

        dlW, _ := r.SmallTextSize("DL")
        dlPillW := dlW + dlBp*2
        dlPillH := smallFH + 4

        delCircleD := smallFH + 4
        delLabelW, _ := r.SmallTextSize("Delete")
        delBlockW := delCircleD + 6 + delLabelW

        cardX := actionEndX + 12
        cardW := r.W - margin - cardX
        cardH := dlPillH + cp*2

        // Align card vertically with the action button.
        cardY := rowY + (rowH-cardH)/2
        if cardY < rowY {
            cardY = rowY
        }

        r.DrawRect(cardX, cardY, cardW, cardH, 42, 42, 58)
        r.DrawRect(cardX+1, cardY+1, cardW-2, cardH-2, 10, 10, 18)

        r.DrawPill(cardX+cp, cardY+cp, dlPillW, dlPillH, 80, 200, 220)
        r.DrawSmallTextCenteredInRect("DL", cardX+cp, cardY+cp, dlPillW, dlPillH, 20, 20, 20)

        delCircleX := cardX + cardW - cp - delBlockW
        delCircleCX := delCircleX + delCircleD/2
        delCircleCY := cardY + cardH/2
        r.DrawCircleBadge(delCircleCX, delCircleCY, delCircleD, 160, 50, 50)
        r.DrawSmallTextCenteredInRect("X", delCircleX, cardY+cp, delCircleD, delCircleD, aT[0], aT[1], aT[2])
        r.DrawSmallText("Delete", delCircleX+delCircleD+6, cardY+cp+2, 200, 80, 80)

        textX := cardX + cp + dlPillW + 6
        textMaxW := delCircleX - textX - 4
        pathW, _ := r.SmallTextSize(pathText)

        r.SetClipRect(textX, cardY, textMaxW, cardH)
        if pathW <= textMaxW {
            s.pathScrollX = 0
            r.DrawSmallText(pathText, textX, cardY+cp+2, 100, 100, 120)
        } else if r.W <= narrowScreenW {
            // On small screens truncate the path rather than scrolling.
            truncated := truncateSmallToWidth(r, pathText, textMaxW)
            r.DrawSmallText(truncated, textX, cardY+cp+2, 100, 100, 120)
        } else {
            maxScrollX := pathW - textMaxW
            scrollX := s.pathScrollX
            if scrollX > maxScrollX {
                scrollX = maxScrollX
            }
            r.DrawSmallText(pathText, textX-scrollX, cardY+cp+2, 100, 100, 120)
            if s.pathScrollX >= maxScrollX {
                totalDur := pathScrollDelay +
                    time.Duration(maxScrollX)*time.Second/time.Duration(pathScrollSpeed) +
                    time.Second
                if time.Since(s.pathScrollAt) > totalDur {
                    s.pathScrollX = 0
                    s.pathScrollAt = time.Now()
                }
            }
        }
        r.SetClipRect(0, contentTop, r.W, contentH)

        // Unified naming toggle (below the combined row, if applicable)
        y = rowY + rowH + 4
        if s.cfg.UnifiedNaming {
            toggleLabel := "Disable title filename"
            if entry.UnifiedNamingDisabled {
                toggleLabel = "Enable title filename"
            }
            drawActionRow("Y", toggleLabel, mt[0], mt[1], mt[2], ac[0], ac[1], ac[2], 0)
        }
    } else {
        y = rowY + rowH + 4
    }
```

Note: the closing `}` for `if isPresent {` stays; the `else {` branch (non-downloaded games) keeps the existing `drawActionRow` calls unchanged.

- [ ] **Step 3: Build**

```bash
./scripts/build.sh native 2>&1 | tail -3
```

- [ ] **Step 4: Commit**

```bash
git add internal/ui/screen_detail.go
git commit -m "feat(detail): combine action button and status card onto one row"
```

---

### Task 6: Detail screen — description with `DrawFormattedText`

**Files:**
- Modify: `internal/ui/screen_detail.go` (description section, ~lines 532–538)

- [ ] **Step 1: Find the description section**

```go
// ── Description (full width) ────────────────────────────
if s.detail != nil && s.detail.Description != "" {
    r.DrawRect(margin, y, usableW, 1, 50, 50, 50) // separator
    y += 11
    descH := r.DrawWrappedText(s.detail.Description, margin, y, usableW, fontH+4, 180, 180, 180)
    y += descH
}
```

- [ ] **Step 2: Replace `DrawWrappedText` with `DrawFormattedText`**

```go
// ── Description (full width, HTML-formatted) ─────────────
if s.detail != nil && s.detail.Description != "" {
    r.DrawRect(margin, y, usableW, 1, 50, 50, 50)
    y += 11
    descH := r.DrawFormattedText(s.detail.Description, margin, y, usableW, fontH+4, 180, 180, 180)
    y += descH
}
```

That's a two-word change: `DrawWrappedText` → `DrawFormattedText`.

- [ ] **Step 3: Build and run tests**

```bash
./scripts/build.sh native 2>&1 | tail -3
./scripts/test.sh 2>&1 | grep -E "ok|FAIL"
```

Expected: clean build, all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/screen_detail.go
git commit -m "feat(detail): render description with DrawFormattedText (paragraphs, headings, lists)"
```

---

### Task 7: Cross-compile and visual verification

- [ ] **Step 1: Cross-compile for all platforms**

```bash
./scripts/build.sh all 2>&1 | tail -5
```

Expected: three ARM64 binaries built cleanly.

- [ ] **Step 2: Release and deploy**

```bash
./scripts/release.sh && ./scripts/deploy.sh
```

- [ ] **Step 3: Capture screenshots and verify key screens**

```bash
./scripts/dev-screenshot.sh --screen detail --out-dir /tmp/detail-redesign
```

Verify in the screenshot:
- Header shows platform badge (e.g. "Pico-8") and "● Downloaded" / "● DL" badge
- Screenshot area is noticeably shorter (~40% height, not 66%)
- QR code is visible but smaller beside the screenshot
- Action button and status card are on the **same** row
- Tags visible without scrolling
- Description starts on screen and shows paragraph spacing
- Description shows headings in bold/bright for games that have them
- Bullet lists show `•` prefix with indent

- [ ] **Step 4: Final commit**

```bash
git add -A  # only if there are uncommitted assets
git status  # verify nothing unexpected
```
