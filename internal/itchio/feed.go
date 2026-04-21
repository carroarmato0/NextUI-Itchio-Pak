package itchio

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

type Game struct {
	Title    string   `json:"title"`
	Author   string   `json:"author"`
	URL      string   `json:"url"`
	CoverURL string   `json:"cover_url"`
	Price    float64  `json:"price"`
	IsFree   bool     `json:"is_free"`
	Tags     []string `json:"tags,omitempty"` // extracted from [Tag] brackets in the RSS title
}

var (
	coverRegex = regexp.MustCompile(`<img[^>]+src="([^"]+)"`)
	tagRegex   = regexp.MustCompile(`\s*\[([^\]]+)\]`)
)

// parseTitle strips [Tag] brackets from the raw RSS title.
func parseTitle(raw string) string {
	return strings.TrimSpace(tagRegex.ReplaceAllString(raw, ""))
}

// parseTags extracts the contents of every [Tag] bracket in the raw RSS title.
func parseTags(raw string) []string {
	matches := tagRegex.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		return nil
	}
	tags := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 && m[1] != "" {
			tags = append(tags, m[1])
		}
	}
	return tags
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	ImageURL    string `xml:"imageurl"`
	Price       string `xml:"price"`
}

type rssFeed struct {
	Items []rssItem `xml:"channel>item"`
}

func parseAuthor(gameURL string) string {
	// https://{author}.itch.io/{game}
	s := strings.TrimPrefix(gameURL, "https://")
	s = strings.TrimPrefix(s, "http://")
	if idx := strings.Index(s, ".itch.io"); idx > 0 {
		return s[:idx]
	}
	return ""
}

func parseCover(imageURL, desc string) string {
	if imageURL != "" {
		return imageURL
	}
	m := coverRegex.FindStringSubmatch(desc)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func parsePrice(raw string) float64 {
	s := strings.TrimSpace(raw)
	// Strip leading currency symbol(s) like "$", "€", "£"
	s = strings.TrimLeft(s, "$€£¥")
	s = strings.TrimSpace(s)
	price, _ := strconv.ParseFloat(s, 64)
	return price
}

func (c *Client) FetchGamesFromURL(url string) ([]Game, error) {
	logger.Debug("feed: fetching %s", url)
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error("feed: HTTP %d from %s", resp.StatusCode, url)
		return nil, fmt.Errorf("fetch feed: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read feed: %w", err)
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		logger.Error("feed: parse XML: %v", err)
		logger.Debug("feed: response body (first 512 bytes): %.512s", body)
		return nil, fmt.Errorf("parse feed xml: %w", err)
	}
	logger.Debug("feed: parsed %d items from XML", len(feed.Items))

	games := make([]Game, 0, len(feed.Items))
	for _, item := range feed.Items {
		price := parsePrice(item.Price)
		games = append(games, Game{
			Title:    parseTitle(item.Title),
			Tags:     parseTags(item.Title),
			Author:   parseAuthor(item.Link),
			URL:      item.Link,
			CoverURL: parseCover(item.ImageURL, item.Description),
			Price:    price,
			IsFree:   price == 0,
		})
	}
	return games, nil
}

const PerPage = 36 // itch.io XML feeds return 36 items per page

func (c *Client) FetchGames(page int, query string) ([]Game, error) {
	url := fmt.Sprintf("%s/games/made-with-gb-studio.xml?page=%d", c.base, page)
	if query != "" {
		url += "&q=" + query
	}
	return c.FetchGamesFromURL(url)
}

// FetchAllGames fetches every page of the GB Studio feed until an empty page
// is returned or ctx is cancelled. progress is called with the running total
// of games fetched after each page (may be nil).
func (c *Client) FetchAllGames(ctx context.Context, progress func(fetched int)) ([]Game, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var (
		all []Game
		mu  sync.Mutex
	)

	fetchPage := func(page int) ([]Game, error) {
		url := fmt.Sprintf("%s/games/made-with-gb-studio.xml?page=%d", c.base, page)
		return c.FetchGamesFromURL(url)
	}

	// Page 1 — always first so we know whether there's anything to fetch.
	games, err := fetchPage(1)
	if err != nil {
		return nil, fmt.Errorf("fetch all games page 1: %w", err)
	}
	all = append(all, games...)
	if progress != nil {
		progress(len(all))
	}
	if len(games) < PerPage {
		return all, nil // single page, done
	}

	// Fetch remaining pages sequentially. The user sees the live feed during
	// this background pass, so total time is acceptable.
	for page := 2; ; page++ {
		select {
		case <-ctx.Done():
			return all, ctx.Err()
		default:
		}

		games, err := fetchPage(page)
		if err != nil {
			logger.Warn("cache: page %d error: %v (stopping early)", page, err)
			break
		}
		mu.Lock()
		all = append(all, games...)
		total := len(all)
		mu.Unlock()
		if progress != nil {
			progress(total)
		}
		if len(games) < PerPage {
			break // last page
		}
	}
	return all, nil
}

var resultCountRegex = regexp.MustCompile(`(?i)(\d[\d,]*)\s+result`)

// FetchTotalGames scrapes the HTML browse page to find the total result count.
func (c *Client) FetchTotalGames() (int, error) {
	logger.Debug("feed: fetching total games count")
	resp, err := c.http.Get("https://itch.io/games/made-with-gb-studio")
	if err != nil {
		return 0, fmt.Errorf("fetch browse page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error("feed: total-games HTTP %d", resp.StatusCode)
		return 0, fmt.Errorf("fetch total games: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read browse page: %w", err)
	}
	m := resultCountRegex.FindStringSubmatch(string(body))
	if len(m) < 2 {
		logger.Warn("feed: result count not found on browse page")
		return 0, fmt.Errorf("result count not found on browse page")
	}
	countStr := strings.ReplaceAll(m[1], ",", "")
	count, err := strconv.Atoi(countStr)
	if err != nil {
		return 0, fmt.Errorf("parse result count: %w", err)
	}
	return count, nil
}
