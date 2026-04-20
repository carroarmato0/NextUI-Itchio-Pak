package itchio

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

type GameDetail struct {
	Game
	Description    string
	ScreenshotURLs []string
	Uploads        []Upload
	GameID         string
	CSRFToken      string
}

type Upload struct {
	Filename string
	URL      string   // resolver or CDN URL
	UploadID string   // itch.io upload ID (from data-upload_id)
}

var (
	// itch:path meta tag: <meta name="itch:path" content="games/850892" />
	gameIDRegex = regexp.MustCompile(`name="itch:path"\s+content="games/(\d+)"`)
	csrfRegex   = regexp.MustCompile(`name="csrf_token"\s+(?:content|value)="([^"]+)"`)
)

func (c *Client) FetchGameDetail(gameURL string) (*GameDetail, error) {
	resp, err := c.http.Get(gameURL)
	if err != nil {
		return nil, fmt.Errorf("fetch game page: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read game page: %w", err)
	}
	s := string(body)

	detail := &GameDetail{}

	if m := gameIDRegex.FindStringSubmatch(s); len(m) > 1 {
		detail.GameID = m[1]
	}
	if m := csrfRegex.FindStringSubmatch(s); len(m) > 1 {
		detail.CSRFToken = m[1]
	}

	// Extract screenshot URLs from screenshot img elements
	screenshotReg := regexp.MustCompile(`class="[^"]*screenshot[^"]*"[^>]+src="([^"]+)"`)
	for _, m := range screenshotReg.FindAllStringSubmatch(s, -1) {
		detail.ScreenshotURLs = append(detail.ScreenshotURLs, m[1])
	}

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
	resp, err := c.http.Get(pageURL)
	if err != nil {
		return nil, fmt.Errorf("fetch download page: %w", err)
	}
	defer resp.Body.Close()

	rawHTML, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read download page: %w", err)
	}

	pageStr := string(rawHTML)
	result := &DownloadPageResult{}

	// Extract CSRF token from the signed page — it is required in the resolver POST body.
	if m := csrfRegex.FindStringSubmatch(pageStr); len(m) > 1 {
		result.CSRFToken = m[1]
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
					result.Uploads = append(result.Uploads, u)
				}
			}
			return // don't recurse into upload divs
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkDoc(c)
		}
	}
	walkDoc(doc)
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
