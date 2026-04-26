package itchio

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// APIKeyStatus is the result of the background API key validation.
type APIKeyStatus int32

const (
	APIKeyStatusUnknown  APIKeyStatus = 0 // not yet tested, or network unavailable
	APIKeyStatusWorking  APIKeyStatus = 1 // accepted by itch.io
	APIKeyStatusRejected APIKeyStatus = 2 // explicitly rejected by itch.io
)

// GetAPIKeyStatus returns the cached result of the most recent key check.
func (c *Client) GetAPIKeyStatus() APIKeyStatus {
	return APIKeyStatus(atomic.LoadInt32(&c.apiKeyStatus))
}

// StoreAPIKeyStatus saves the result of a completed key check.
func (c *Client) StoreAPIKeyStatus(s APIKeyStatus) {
	atomic.StoreInt32(&c.apiKeyStatus, int32(s))
}

// MarkAPIKeyCheckStarted atomically marks the background check as started.
// Returns true only on the first call — the caller should then launch the check.
func (c *Client) MarkAPIKeyCheckStarted() bool {
	return atomic.CompareAndSwapInt32(&c.apiKeyChecking, 0, 1)
}

// CheckAPIKey does a lightweight /profile fetch to determine whether apiKey is
// accepted. Returns APIKeyStatusWorking on success, APIKeyStatusRejected when
// the server explicitly rejects the key, and APIKeyStatusUnknown on network or
// other transient errors (so the UI can show "PRESENT" rather than "REJECTED").
func (c *Client) CheckAPIKey(apiKey string) APIKeyStatus {
	req, err := http.NewRequest("GET", c.butler+"/profile", nil)
	if err != nil {
		return APIKeyStatusUnknown
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		logger.Debug("validate: background key check network error: %v", err)
		return APIKeyStatusUnknown
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return APIKeyStatusWorking
	case http.StatusUnauthorized, http.StatusForbidden:
		logger.Debug("validate: background key check rejected (HTTP %d)", resp.StatusCode)
		return APIKeyStatusRejected
	default:
		logger.Debug("validate: background key check unexpected HTTP %d", resp.StatusCode)
		return APIKeyStatusUnknown
	}
}

// OwnedGame is a public summary of a game the user owns.
// Download key IDs are never included here — they grant download access and
// must not appear in logs.
type OwnedGame struct {
	GameID int64
	Title  string
	URL    string
}

// obfuscateName keeps the first and last rune visible and replaces every
// character in between with '*', so names are recognisable but not logged in full.
func obfuscateName(name string) string {
	r := []rune(name)
	if len(r) <= 2 {
		return string(r)
	}
	out := make([]rune, len(r))
	out[0] = r[0]
	out[len(r)-1] = r[len(r)-1]
	for i := 1; i < len(r)-1; i++ {
		if r[i] == ' ' {
			out[i] = ' '
		} else {
			out[i] = '*'
		}
	}
	return string(out)
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
	logger.Info("validate: authenticated as %q", obfuscateName(username))

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
