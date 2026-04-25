package itchio

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

const apiItchIO = "https://api.itch.io"

// FetchAuthUploads lists .gb/.gbc uploads for a paid game the user owns.
//
// Flow:
//  1. GET api.itch.io/profile/owned-keys?game_id=GAME_ID  → buyer's download key ID
//  2. GET itch.io/api/1/KEY/game/GAME_ID/uploads?download_key_id=KEY_ID → upload list
func (c *Client) FetchAuthUploads(apiKey, gameID string) ([]Upload, string, error) {
	// Step 1: get buyer's download key ID from the butler-style API.
	keysURL := fmt.Sprintf("%s/profile/owned-keys?game_id=%s", c.butler, gameID)
	logger.Debug("auth: fetching owned keys for game_id=%s", gameID)
	req, err := http.NewRequest("GET", keysURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build owned-keys request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch owned keys: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error("auth: owned-keys HTTP %d", resp.StatusCode)
		return nil, "", fmt.Errorf("fetch owned keys: HTTP %d", resp.StatusCode)
	}

	var keysResult struct {
		OwnedKeys []struct {
			ID int64 `json:"id"`
		} `json:"owned_keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&keysResult); err != nil {
		return nil, "", fmt.Errorf("decode owned keys: %w", err)
	}
	if len(keysResult.OwnedKeys) == 0 {
		return nil, "", fmt.Errorf("game not owned or API key invalid (no download key found)")
	}
	if keysResult.OwnedKeys[0].ID == 0 {
		logger.Warn("auth: owned-keys returned entry with id=0 (game not owned or API issue)")
		return nil, "", fmt.Errorf("game not owned or API key invalid (no valid download key)")
	}
	downloadKeyID := fmt.Sprintf("%d", keysResult.OwnedKeys[0].ID)
	// downloadKeyID not logged — it ties the request to the user's account.

	// Step 2: list uploads, passing the download key so itch.io grants access.
	uploadsURL := fmt.Sprintf("%s/api/1/%s/game/%s/uploads?download_key_id=%s",
		c.base, apiKey, gameID, downloadKeyID)
	logger.Debug("auth: fetching upload list for game_id=%s", gameID)
	// The URL contains the API key; the logger's secret registry will redact it.

	resp2, err := c.http.Get(uploadsURL)
	if err != nil {
		return nil, "", fmt.Errorf("fetch uploads: %w", err)
	}
	defer resp2.Body.Close()

	rawBody, err := io.ReadAll(resp2.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read uploads response: %w", err)
	}

	if resp2.StatusCode != http.StatusOK {
		logger.Error("auth: upload list HTTP %d: %.200s", resp2.StatusCode, rawBody)
		return nil, "", fmt.Errorf("fetch uploads: HTTP %d", resp2.StatusCode)
	}

	var uploadsResult struct {
		Uploads []struct {
			ID       int64  `json:"id"`
			Filename string `json:"filename"`
		} `json:"uploads"`
	}
	if err := json.Unmarshal(rawBody, &uploadsResult); err != nil {
		logger.Error("auth: decode uploads: %v (body: %.200s)", err, rawBody)
		return nil, "", fmt.Errorf("decode uploads: %w", err)
	}

	var uploads []Upload
	for _, u := range uploadsResult.Uploads {
		ext := strings.ToLower(filepath.Ext(u.Filename))
		if ext == ".gb" || ext == ".gbc" {
			uploads = append(uploads, Upload{
				Filename: u.Filename,
				UploadID: fmt.Sprintf("%d", u.ID),
			})
			logger.Debug("auth: found ROM %s id=%d", u.Filename, u.ID)
		} else if !isSkippableExt(ext) {
			uploads = append(uploads, Upload{
				Filename:    u.Filename,
				UploadID:    fmt.Sprintf("%d", u.ID),
				NeedsFormat: true,
			})
			logger.Debug("auth: found unknown-format %s id=%d (user will choose format)", u.Filename, u.ID)
		} else {
			logger.Debug("auth: skipping %s (ext=%q)", u.Filename, ext)
		}
	}
	knownCount := 0
	for _, u := range uploads {
		if !u.NeedsFormat {
			knownCount++
		}
	}
	logger.Debug("auth: %d known ROM(s), %d unknown-format file(s) from %d total",
		knownCount, len(uploads)-knownCount, len(uploadsResult.Uploads))
	if len(uploads) == 0 {
		logger.Warn("auth: no downloadable uploads found (game has %d uploads, all skipped)",
			len(uploadsResult.Uploads))
	}
	return uploads, downloadKeyID, nil
}

// DownloadAuthUpload resolves the CDN URL for an owned upload and streams it to dest.
func (c *Client) DownloadAuthUpload(apiKey, uploadID, downloadKeyID, dest string, progress func(int64, int64)) error {
	dlURL := fmt.Sprintf("%s/api/1/%s/upload/%s/download?download_key_id=%s",
		c.base, apiKey, uploadID, downloadKeyID)
	logger.Debug("auth: resolving CDN for upload id=%s", uploadID)
	// dlURL contains the API key — the logger's secret registry will redact it if logged.

	resp, err := c.http.Get(dlURL)
	if err != nil {
		return fmt.Errorf("resolve auth CDN URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error("auth: CDN resolve HTTP %d", resp.StatusCode)
		return fmt.Errorf("auth CDN resolve status %d", resp.StatusCode)
	}

	var result struct {
		URL    string   `json:"url"`
		Errors []string `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode auth CDN response: %w", err)
	}
	if len(result.Errors) > 0 {
		logger.Error("auth: CDN error: %s", strings.Join(result.Errors, "; "))
		return fmt.Errorf("auth CDN error: %s", strings.Join(result.Errors, "; "))
	}
	if result.URL == "" {
		logger.Error("auth: empty CDN URL from resolver")
		return fmt.Errorf("empty CDN URL from auth resolver")
	}

	logger.Info("auth: streaming to %s", dest)
	return c.streamToFile(result.URL, dest, progress)
}
