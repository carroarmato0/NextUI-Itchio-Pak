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
//  2. GET api.itch.io/games/GAME_ID/uploads?download_key_id=KEY_ID → upload list
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

	// Step 2: list uploads using the butler API with the download key to prove ownership.
	req2, err := http.NewRequest("GET",
		fmt.Sprintf("%s/games/%s/uploads?download_key_id=%s", c.butler, gameID, downloadKeyID),
		nil)
	if err != nil {
		return nil, "", fmt.Errorf("build uploads request: %w", err)
	}
	req2.Header.Set("Authorization", "Bearer "+apiKey)
	logger.Debug("auth: fetching upload list for game_id=%s", gameID)

	resp2, err := c.http.Do(req2)
	if err != nil {
		return nil, "", fmt.Errorf("fetch uploads: %w", err)
	}
	defer resp2.Body.Close()

	rawBody, err := io.ReadAll(resp2.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read uploads response: %w", err)
	}

	if resp2.StatusCode == http.StatusForbidden || resp2.StatusCode == http.StatusUnauthorized {
		logger.Warn("auth: upload list HTTP %d — game not owned or API key lacks access", resp2.StatusCode)
		return nil, "", fmt.Errorf("game not owned or API key does not grant access to this game's downloads")
	}
	// itch.io returns 500 when the download_key_id is not valid for this game
	// (e.g. the owned-keys endpoint returned a key that doesn't grant access).
	// Treat this as a "not owned" condition rather than a generic server error.
	if resp2.StatusCode == http.StatusInternalServerError {
		logger.Warn("auth: upload list HTTP 500 — download key likely invalid; game may not be owned (body: %.200s)", rawBody)
		return nil, "", fmt.Errorf("Game not owned or download key invalid — purchase this game to download it")
	}
	if resp2.StatusCode != http.StatusOK {
		logger.Error("auth: upload list HTTP %d: %.200s", resp2.StatusCode, rawBody)
		return nil, "", fmt.Errorf("fetch uploads: HTTP %d", resp2.StatusCode)
	}

	// itch.io returns {"uploads":[...]} when uploads are accessible, but
	// {"uploads":{}} (an object, not an array) when there are none or access is
	// restricted. Decode the uploads field as raw JSON so we can handle both.
	var envelope struct {
		Uploads json.RawMessage `json:"uploads"`
	}
	if err := json.Unmarshal(rawBody, &envelope); err != nil {
		logger.Error("auth: decode uploads response: %v (body: %.200s)", err, rawBody)
		return nil, "", fmt.Errorf("decode uploads response: %w", err)
	}

	var uploadItems []struct {
		ID       int64  `json:"id"`
		Filename string `json:"filename"`
	}
	if len(envelope.Uploads) > 0 && envelope.Uploads[0] == '[' {
		if err := json.Unmarshal(envelope.Uploads, &uploadItems); err != nil {
			logger.Error("auth: decode uploads array: %v", err)
			return nil, "", fmt.Errorf("decode uploads: %w", err)
		}
	} else {
		logger.Debug("auth: uploads field is not an array (got %.50s) — treating as empty", envelope.Uploads)
	}

	var uploads []Upload
	for _, u := range uploadItems {
		ext := strings.ToLower(filepath.Ext(u.Filename))
		if ext == ".gb" || ext == ".gbc" || ext == ".zip" {
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
		knownCount, len(uploads)-knownCount, len(uploadItems))
	if len(uploads) == 0 {
		logger.Warn("auth: no downloadable uploads found (game has %d uploads, all skipped)",
			len(uploadItems))
	}
	return uploads, downloadKeyID, nil
}

// DownloadAuthUpload resolves the CDN URL for an owned upload and streams it to dest.
func (c *Client) DownloadAuthUpload(apiKey, uploadID, downloadKeyID, dest string, progress func(int64, int64)) error {
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/uploads/%s/download?download_key_id=%s", c.butler, uploadID, downloadKeyID),
		nil)
	if err != nil {
		return fmt.Errorf("build auth download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	logger.Debug("auth: resolving CDN for upload id=%s", uploadID)

	resp, err := c.http.Do(req)
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
