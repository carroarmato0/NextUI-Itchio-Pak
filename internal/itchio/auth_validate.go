package itchio

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// OwnedGame is a public summary of a game the user owns.
// Download key IDs are never included here — they grant download access and
// must not appear in logs.
type OwnedGame struct {
	GameID int64
	Title  string
	URL    string
}

// ValidateAPIKey checks that apiKey is valid by fetching the caller's itch.io
// profile, then pages through all owned-game keys and returns the account
// username and the full owned-game list.
//
// Each owned game title and public game ID are logged at DEBUG level.
// Download key IDs are never logged.
func (c *Client) ValidateAPIKey(apiKey string) (username string, owned []OwnedGame, err error) {
	// Step 1: verify key and fetch username.
	req, err := http.NewRequest("GET", c.butler+"/profile", nil)
	if err != nil {
		return "", nil, fmt.Errorf("build profile request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("fetch profile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return "", nil, fmt.Errorf("API key invalid or expired (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("fetch profile: HTTP %d", resp.StatusCode)
	}

	var profileResp struct {
		User struct {
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profileResp); err != nil {
		return "", nil, fmt.Errorf("decode profile: %w", err)
	}
	username = profileResp.User.Username
	if profileResp.User.DisplayName != "" {
		username = profileResp.User.DisplayName
	}
	logger.Info("validate: authenticated as %q", username)

	// Step 2: page through all owned-game keys.
	// Each entry carries a download key ID (never logged) and a public game object.
	for page := 1; page <= 20; page++ { // cap: 20 pages × 10 = 200 games
		req, err := http.NewRequest("GET",
			fmt.Sprintf("%s/profile/owned-keys?page=%d", c.butler, page), nil)
		if err != nil {
			logger.Warn("validate: build owned-keys request page %d: %v", page, err)
			break
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := c.http.Do(req)
		if err != nil {
			logger.Warn("validate: owned-keys page %d: %v", page, err)
			break
		}

		// itch.io returns {"owned_keys":{}} (object, not array) on the last page
		// when there are no more entries. Use RawMessage to handle both cases.
		var envelope struct {
			OwnedKeys json.RawMessage `json:"owned_keys"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&envelope)
		resp.Body.Close()
		if decodeErr != nil {
			logger.Warn("validate: decode owned-keys page %d: %v", page, decodeErr)
			break
		}

		if len(envelope.OwnedKeys) == 0 || envelope.OwnedKeys[0] != '[' {
			break // empty object or unexpected format — no more pages
		}

		var keyItems []struct {
			Game struct {
				ID    int64  `json:"id"`
				Title string `json:"title"`
				URL   string `json:"url"`
			} `json:"game"`
		}
		if err := json.Unmarshal(envelope.OwnedKeys, &keyItems); err != nil {
			logger.Warn("validate: unmarshal owned-keys page %d: %v", page, err)
			break
		}

		if len(keyItems) == 0 {
			break
		}
		for _, k := range keyItems {
			g := OwnedGame{GameID: k.Game.ID, Title: k.Game.Title, URL: k.Game.URL}
			owned = append(owned, g)
			logger.Debug("validate: owned game id=%d %q", g.GameID, g.Title)
		}
	}

	logger.Info("validate: %d owned game(s) found", len(owned))
	return username, owned, nil
}
