package itchio

import (
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

type Game struct {
	Title    string
	Author   string
	URL      string
	CoverURL string
	Price    float64
	IsFree   bool
}

var coverRegex = regexp.MustCompile(`<img[^>]+src="([^"]+)"`)

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
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch feed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read feed: %w", err)
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse feed xml: %w", err)
	}

	games := make([]Game, 0, len(feed.Items))
	for _, item := range feed.Items {
		price := parsePrice(item.Price)
		games = append(games, Game{
			Title:    item.Title,
			Author:   parseAuthor(item.Link),
			URL:      item.Link,
			CoverURL: parseCover(item.ImageURL, item.Description),
			Price:    price,
			IsFree:   price == 0,
		})
	}
	return games, nil
}

func (c *Client) FetchGames(page int, query string) ([]Game, error) {
	url := fmt.Sprintf("https://itch.io/games/made-with-gb-studio.xml?page=%d", page)
	if query != "" {
		url += "&q=" + query
	}
	return c.FetchGamesFromURL(url)
}
