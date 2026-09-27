package itchio

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
)

// GameData is a game's https://{author}.itch.io/{game}/data.json: public, no
// sign-in needed, and extended by itch.io for this app (issue #4). It replaces
// scraping the game page for the ID, tags, screenshots and price, and the
// /purchase page for the suggested amount.
type GameData struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	// Price is empty when payments are disabled (free), "$0.00" for
	// name-your-own-price, and the minimum price for a paid game.
	Price string `json:"price"`
	// SuggestedPrice is the developer's suggested amount; empty if none.
	SuggestedPrice string `json:"suggested_price"`
	// OriginalPrice and Sale are set only while the game is on sale.
	OriginalPrice string    `json:"original_price"`
	Sale          *GameSale `json:"sale"`

	Screenshots []string     `json:"screenshots"`
	Tags        []string     `json:"tags"`
	CoverImage  string       `json:"cover_image"`
	Authors     []GameAuthor `json:"authors"`

	// URL is the game's own address. After a rename it differs from the one
	// that was asked for, which itch.io redirected.
	URL string `json:"-"`

	Links struct {
		Self string `json:"self"`
	} `json:"links"`
}

// GameSale describes a running sale.
type GameSale struct {
	Title   string `json:"title"`
	Rate    int    `json:"rate"`     // percent off
	EndDate string `json:"end_date"` // as itch.io gives it, UTC
}

// GameAuthor is one of a game's authors.
type GameAuthor struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Pricing classifies the game the way the rest of the app does.
func (d *GameData) Pricing() PricingModel {
	switch {
	case d.Price == "":
		return PricingFree
	case isZeroAmount(d.Price):
		return PricingNameYourOwnPrice
	default:
		return PricingPaid
	}
}

// dataJSONMaxBytes bounds a data.json body; real ones are a few KiB.
const dataJSONMaxBytes = 1 << 20

// FetchGameData reads a game's data.json. A missing game (404/410) returns
// ErrGameRemoved; a renamed one is followed, and URL says where it moved.
func (c *Client) FetchGameData(gameURL string) (*GameData, error) {
	u := strings.TrimRight(gameURL, "/") + "/data.json"
	logger.Debug("game: fetching data.json for %s", gameURL)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build data.json request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch data.json: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		logger.Info("game: data.json HTTP %d — game removed: %s", resp.StatusCode, gameURL)
		return nil, ErrGameRemoved
	}
	if resp.StatusCode != http.StatusOK {
		logger.Warn("game: data.json HTTP %d for %s", resp.StatusCode, gameURL)
		return nil, &netstate.StatusError{What: "fetch data.json", Code: resp.StatusCode}
	}
	// Read the whole body before decoding, so the two failures stay apart: a
	// read error is the connection dropping (offline-class), while a body that
	// arrived complete but is not valid JSON, or is too big, is a bad response.
	// A streaming decoder reports a short body as io.ErrUnexpectedEOF, which
	// netstate counts as offline, and the update check would then flap
	// offline/online on the same game after every reconnect.
	body, err := io.ReadAll(io.LimitReader(resp.Body, dataJSONMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read data.json: %w", err)
	}
	if len(body) > dataJSONMaxBytes {
		logger.Warn("game: data.json for %s is larger than %d bytes, ignored", gameURL, dataJSONMaxBytes)
		return nil, fmt.Errorf("data.json is larger than %d bytes", dataJSONMaxBytes)
	}
	var d GameData
	if err := json.Unmarshal(body, &d); err != nil {
		logger.Warn("game: data.json for %s is not valid JSON (%d bytes): %v", gameURL, len(body), err)
		return nil, fmt.Errorf("decode data.json: %w", err)
	}
	d.URL = d.Links.Self
	if d.URL == "" {
		d.URL = gameURL
	}
	if d.URL != gameURL {
		logger.Info("game: %s now lives at %s", gameURL, d.URL)
	}
	logger.Debug("game: data.json id=%d pricing=%d price=%q suggested=%q sale=%v screenshots=%d tags=%d",
		d.ID, d.Pricing(), d.Price, d.SuggestedPrice, d.Sale != nil, len(d.Screenshots), len(d.Tags))
	return &d, nil
}
