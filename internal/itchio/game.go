package itchio

import (
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
	URL      string
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

	return detail, nil
}

func (c *Client) ParseDownloadPage(pageURL string) ([]Upload, error) {
	resp, err := c.http.Get(pageURL)
	if err != nil {
		return nil, fmt.Errorf("fetch download page: %w", err)
	}
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse download page: %w", err)
	}

	var uploads []Upload
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					href := attr.Val
					// Strip query string to get clean filename
					base := strings.Split(href, "?")[0]
					ext := strings.ToLower(filepath.Ext(base))
					if ext == ".gb" || ext == ".gbc" {
						filename := filepath.Base(base)
						uploads = append(uploads, Upload{Filename: filename, URL: href})
					}
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return uploads, nil
}
