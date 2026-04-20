package itchio

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
)

const apiItchIO = "https://api.itch.io"

// FetchAuthUploads lists .gb/.gbc uploads for a paid game the user owns.
//
// Flow:
//  1. GET api.itch.io/profile/owned-keys?game_id=GAME_ID  → buyer's download key ID
//  2. GET itch.io/api/1/KEY/game/GAME_ID/uploads?download_key_id=KEY_ID → upload list
func (c *Client) FetchAuthUploads(apiKey, gameID string) ([]Upload, string, error) {
	// Step 1: get buyer's download key ID from the butler-style API.
	// This endpoint requires Authorization: Bearer and returns the purchase key
	// associated with the authenticated user's ownership of the game.
	keysURL := fmt.Sprintf("%s/profile/owned-keys?game_id=%s", c.butler, gameID)
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
	downloadKeyID := fmt.Sprintf("%d", keysResult.OwnedKeys[0].ID)
	log.Printf("FetchAuthUploads: download_key_id=%s", downloadKeyID)

	// Step 2: list uploads, passing the download key so itch.io grants access.
	uploadsURL := fmt.Sprintf("%s/api/1/%s/game/%s/uploads?download_key_id=%s",
		c.base, apiKey, gameID, downloadKeyID)
	resp2, err := c.http.Get(uploadsURL)
	if err != nil {
		return nil, "", fmt.Errorf("fetch uploads: %w", err)
	}
	defer resp2.Body.Close()

	var uploadsResult struct {
		Uploads []struct {
			ID       int64  `json:"id"`
			Filename string `json:"filename"`
		} `json:"uploads"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&uploadsResult); err != nil {
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
			log.Printf("FetchAuthUploads: upload %q id=%d", u.Filename, u.ID)
		}
	}
	return uploads, downloadKeyID, nil
}

// DownloadAuthUpload resolves the CDN URL for an owned upload and streams it to dest.
func (c *Client) DownloadAuthUpload(apiKey, uploadID, downloadKeyID, dest string, progress func(int64, int64)) error {
	dlURL := fmt.Sprintf("%s/api/1/%s/upload/%s/download?download_key_id=%s",
		c.base, apiKey, uploadID, downloadKeyID)
	log.Printf("DownloadAuthUpload: resolving CDN URL from upload %s key %s", uploadID, downloadKeyID)

	resp, err := c.http.Get(dlURL)
	if err != nil {
		return fmt.Errorf("resolve auth CDN URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
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
		return fmt.Errorf("auth CDN error: %s", strings.Join(result.Errors, "; "))
	}
	if result.URL == "" {
		return fmt.Errorf("empty CDN URL from auth resolver")
	}

	log.Printf("DownloadAuthUpload: streaming to %s", dest)
	return c.streamToFile(result.URL, dest, progress)
}
