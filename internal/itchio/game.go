package itchio

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"golang.org/x/net/html"
)

type GameDetail struct {
	Game
	Description    string
	ScreenshotURLs []string
	Uploads        []Upload
	GameID         string
	CSRFToken      string
	PageTags       []string // itch.io tag labels scraped from the game page
}

type Upload struct {
	Filename    string
	URL         string // resolver or CDN URL
	UploadID    string // itch.io upload ID (from data-upload_id)
	NeedsFormat bool   // true if extension unknown; user must choose GB, GBC, or ZIP
}

var (
	// itch:path meta tag — attribute order varies across pages, so we match
	// the whole <meta> element containing "itch:path" and extract the game ID
	// from its content attribute in a second pass.
	gameIDTagRegex   = regexp.MustCompile(`<meta[^>]+itch:path[^>]+>`)
	gameIDValueRegex = regexp.MustCompile(`content="games/(\d+)"`)
	csrfRegex        = regexp.MustCompile(`name="csrf_token"\s+(?:content|value)="([^"]+)"`)
	// tag links: <a href="https://itch.io/games/tag-horror">Horror</a>
	// Capture the slug from the URL (e.g. "horror", "lgbtq") for reliable filter matching.
	pageTagRegex = regexp.MustCompile(`href="https://itch\.io/games/tag-([^"]+)"`)
)

func (c *Client) FetchGameDetail(gameURL string) (*GameDetail, error) {
	logger.Debug("game: fetching detail %s", gameURL)
	resp, err := c.http.Get(gameURL)
	if err != nil {
		return nil, fmt.Errorf("fetch game page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error("game: detail page HTTP %d for %s", resp.StatusCode, gameURL)
		return nil, fmt.Errorf("fetch game page: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read game page: %w", err)
	}
	s := string(body)

	detail := &GameDetail{}

	if tag := gameIDTagRegex.FindString(s); tag != "" {
		if m := gameIDValueRegex.FindStringSubmatch(tag); len(m) > 1 {
			detail.GameID = m[1]
		}
	}
	if detail.GameID == "" {
		logger.Warn("game: gameID not found on page (paid download will not work)")
	} else {
		logger.Debug("game: gameID=%q", detail.GameID)
	}

	if m := csrfRegex.FindStringSubmatch(s); len(m) > 1 {
		detail.CSRFToken = m[1]
	} else {
		logger.Warn("game: CSRF token not found on page")
	}

	// Extract itch.io page tags from tag links
	for _, m := range pageTagRegex.FindAllStringSubmatch(s, -1) {
		if len(m) > 1 {
			detail.PageTags = append(detail.PageTags, strings.TrimSpace(m[1]))
		}
	}
	logger.Debug("game: %d page tags: %v", len(detail.PageTags), detail.PageTags)

	// Extract screenshot URLs from screenshot img elements
	screenshotReg := regexp.MustCompile(`class="[^"]*screenshot[^"]*"[^>]+src="([^"]+)"`)
	for _, m := range screenshotReg.FindAllStringSubmatch(s, -1) {
		detail.ScreenshotURLs = append(detail.ScreenshotURLs, m[1])
	}
	logger.Debug("game: %d screenshots found", len(detail.ScreenshotURLs))

	// Extract game description from formatted_description div
	detail.Description = extractDescription(s)

	return detail, nil
}

// descriptionRegex matches the formatted_description div and its contents.
var descriptionRegex = regexp.MustCompile(`(?s)<div\s+class="formatted_description[^"]*">(.*?)</div>`)

// extractDescription pulls the game description from the page HTML and
// converts it from HTML to plain text.
func extractDescription(pageHTML string) string {
	m := descriptionRegex.FindStringSubmatch(pageHTML)
	if len(m) < 2 {
		return ""
	}
	return htmlToPlainText(m[1])
}

// htmlToPlainText converts simple HTML (paragraphs, breaks, headings) to
// readable plain text.
func htmlToPlainText(fragment string) string {
	doc, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		return ""
	}
	var buf bytes.Buffer
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			buf.WriteString(n.Data)
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "br":
				buf.WriteString("\n")
			case "p", "h1", "h2", "h3", "h4", "h5", "h6":
				if buf.Len() > 0 {
					buf.WriteString("\n\n")
				}
			case "li":
				buf.WriteString("\n• ")
			case "tr":
				buf.WriteString("\n")
			case "td", "th":
				if buf.Len() > 0 {
					last := buf.Bytes()[buf.Len()-1]
					if last != '\n' {
						buf.WriteString("  ")
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	// Clean up: collapse excessive newlines, trim
	text := buf.String()
	text = strings.ReplaceAll(text, "\r", "")
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(text)
}

// DownloadPageResult holds what ParseDownloadPage extracts from the signed page.
type DownloadPageResult struct {
	Uploads   []Upload
	CSRFToken string // CSRF token from the signed download page (needed for file resolver POST)
}

// ParseDownloadPage fetches the itch.io signed download page and returns all
// .gb/.gbc uploads found (with UploadID set) plus the page's CSRF token.
// The CSRF token must be included in the body of the subsequent file resolver POST.
func (c *Client) ParseDownloadPage(pageURL string) (*DownloadPageResult, error) {
	// The signed URL contains a download key — do not log it.
	logger.Debug("download-page: fetching signed download page")
	resp, err := c.http.Get(pageURL)
	if err != nil {
		return nil, fmt.Errorf("fetch download page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error("download-page: HTTP %d", resp.StatusCode)
		return nil, fmt.Errorf("fetch download page: HTTP %d", resp.StatusCode)
	}

	rawHTML, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read download page: %w", err)
	}

	pageStr := string(rawHTML)
	result := &DownloadPageResult{}

	// Extract CSRF token — log presence only, never the value.
	if m := csrfRegex.FindStringSubmatch(pageStr); len(m) > 1 {
		result.CSRFToken = m[1]
		logger.Debug("download-page: CSRF token present")
	} else {
		logger.Warn("download-page: CSRF token not found (resolver POST may fail)")
	}

	doc, err := html.Parse(bytes.NewReader(rawHTML))
	if err != nil {
		return nil, fmt.Errorf("parse download page: %w", err)
	}

	var walkDoc func(*html.Node)
	walkDoc = func(n *html.Node) {
		// Find each <div class="upload"> and extract upload info from it
		if n.Type == html.ElementNode && n.Data == "div" && nodeHasClass(n, "upload") {
			if u, ok := extractUploadEntry(n); ok {
				ext := strings.ToLower(filepath.Ext(u.Filename))
				if ext == ".gb" || ext == ".gbc" {
					logger.Debug("download-page: found ROM %s id=%s", u.Filename, u.UploadID)
					result.Uploads = append(result.Uploads, u)
				} else if !isSkippableExt(ext) {
					u.NeedsFormat = true
					logger.Debug("download-page: found unknown-format %s id=%s (user will choose format)", u.Filename, u.UploadID)
					result.Uploads = append(result.Uploads, u)
				} else {
					logger.Debug("download-page: skipping %s (ext=%q)", u.Filename, ext)
				}
			}
			return // don't recurse into upload divs
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkDoc(c)
		}
	}
	walkDoc(doc)
	knownCount := 0
	for _, u := range result.Uploads {
		if !u.NeedsFormat {
			knownCount++
		}
	}
	logger.Info("download-page: %d known ROM(s), %d unknown-format file(s)",
		knownCount, len(result.Uploads)-knownCount)
	return result, nil
}

// extractUploadEntry walks a <div class="upload"> node and extracts the
// filename (from <strong class="name">) and upload ID (from data-upload_id).
func extractUploadEntry(div *html.Node) (Upload, bool) {
	var uploadID, filename string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Data == "a" {
				for _, a := range n.Attr {
					if a.Key == "data-upload_id" && a.Val != "" {
						uploadID = a.Val
					}
				}
			}
			if n.Data == "strong" && nodeHasClass(n, "name") {
				// Prefer title attribute; fall back to text content
				for _, a := range n.Attr {
					if a.Key == "title" && a.Val != "" {
						filename = a.Val
					}
				}
				if filename == "" && n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
					filename = strings.TrimSpace(n.FirstChild.Data)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(div)
	if uploadID == "" || filename == "" {
		return Upload{}, false
	}
	return Upload{Filename: filename, UploadID: uploadID}, true
}

// nodeHasClass reports whether an element node has the given CSS class.
func nodeHasClass(n *html.Node, class string) bool {
	for _, a := range n.Attr {
		if a.Key == "class" {
			for _, c := range strings.Fields(a.Val) {
				if c == class {
					return true
				}
			}
		}
	}
	return false
}
