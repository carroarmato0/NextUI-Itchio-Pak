package itchio

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// FetchAuthUploads lists .gb/.gbc uploads for a paid game the user owns.
//
// gameURL is the public game page URL (e.g. "https://author.itch.io/game"); it
// is used as a fallback to construct the signed download URL when the owned-keys
// response includes a key string but no downloads_url.
//
// Flow:
//  1. GET api.itch.io/profile/owned-keys?game_id=GAME_ID
//  2a. downloads_url present → fetchAuthUploadsViaSignedPage
//  2b. key string present    → construct signed URL from gameURL + key string
//  2c. only numeric ID       → bundle purchase; not downloadable via API
func (c *Client) FetchAuthUploads(apiKey, gameID, gameURL string) ([]Upload, string, error) {
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
			ID           int64  `json:"id"`
			Key          string `json:"key"`          // alphanumeric key string (may be present)
			DownloadsURL string `json:"downloads_url"` // pre-signed page URL (preferred)
		} `json:"owned_keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&keysResult); err != nil {
		return nil, "", fmt.Errorf("decode owned keys: %w", err)
	}
	if len(keysResult.OwnedKeys) == 0 {
		return nil, "", fmt.Errorf("game not owned or API key invalid (no download key found)")
	}

	key := keysResult.OwnedKeys[0]
	logger.Debug("auth: owned-keys: id=%d downloads_url=%s key_str=%s",
		key.ID, presentAbsent(key.DownloadsURL), presentAbsent(key.Key))

	// 2a. Prefer downloads_url: pre-signed page URL that works for direct purchases.
	if key.DownloadsURL != "" {
		logger.Debug("auth: using signed-page path via downloads_url")
		return c.fetchAuthUploadsViaSignedPage(key.DownloadsURL)
	}

	// 2b. If the key string is present and we have the game URL, construct the
	// signed page URL ourselves.
	if key.Key != "" && gameURL != "" {
		signedURL := strings.TrimRight(gameURL, "/") + "/download/" + url.PathEscape(key.Key)
		logger.Debug("auth: constructing signed URL from key string + game URL")
		return c.fetchAuthUploadsViaSignedPage(signedURL)
	}

	if key.ID == 0 {
		logger.Warn("auth: owned-keys returned id=0, no downloads_url, no key string")
		return nil, "", fmt.Errorf("game not owned or API key invalid (no valid download key)")
	}

	// 2c. Only a numeric bundle key ID is present — itch.io does not expose
	// per-game download info for bundle purchases through the API. The user
	// must download via the itch.io website or app.
	logger.Warn("auth: game_id=%s owned via bundle (id=%d) — no per-game download key available",
		gameID, key.ID)
	return nil, "", fmt.Errorf(
		"This game was purchased via a bundle. Bundle downloads are not supported — " +
			"please use the itch.io website or app to download.")
}

// fetchAuthUploadsViaSignedPage uses the signed downloads_url from the owned-keys
// response to list uploads via ParseDownloadPage.
func (c *Client) fetchAuthUploadsViaSignedPage(downloadsURL string) ([]Upload, string, error) {
	key := extractDownloadKey(downloadsURL)
	if key == "" {
		return nil, "", fmt.Errorf("auth: could not extract key from downloads_url")
	}

	// Derive base game URL by stripping "/download/KEY" from the path.
	// e.g. "https://author.itch.io/game/download/KEY" → "https://author.itch.io/game"
	parsed, err := url.Parse(downloadsURL)
	if err != nil {
		return nil, "", fmt.Errorf("auth: parse downloads_url: %w", err)
	}
	const marker = "/download/"
	idx := strings.LastIndex(parsed.EscapedPath(), marker)
	if idx < 0 {
		return nil, "", fmt.Errorf("auth: downloads_url missing /download/ segment")
	}
	base := parsed.Scheme + "://" + parsed.Host + parsed.EscapedPath()[:idx]

	dlPage, err := c.ParseDownloadPage(downloadsURL)
	if err != nil {
		return nil, "", fmt.Errorf("auth: parse signed download page: %w", err)
	}

	var uploads []Upload
	for _, u := range dlPage.Uploads {
		resolverURL := base + "/file/" + u.UploadID +
			"?key=" + url.QueryEscape(key) +
			"&csrf=" + url.QueryEscape(dlPage.CSRFToken)
		logger.Debug("auth: found %s id=%s (signed-page path)", u.Filename, u.UploadID)
		uploads = append(uploads, Upload{
			Filename:    u.Filename,
			UploadID:    u.UploadID,
			URL:         resolverURL,
			NeedsFormat: u.NeedsFormat,
		})
	}
	knownCount := 0
	for _, u := range uploads {
		if !u.NeedsFormat {
			knownCount++
		}
	}
	logger.Debug("auth: %d known ROM(s), %d unknown-format file(s) (signed-page path)",
		knownCount, len(uploads)-knownCount)
	if len(uploads) == 0 {
		logger.Warn("auth: no downloadable uploads found via signed-page path")
	}
	// Empty keyID: callers use DownloadFree (not DownloadAuthUpload) for these uploads.
	return uploads, "", nil
}

// DownloadAuthUpload resolves the CDN URL for an owned upload and streams it to dest.
// Uses the simple API (itch.io/api/1) with a per-game download key.
func (c *Client) DownloadAuthUpload(apiKey, uploadID, downloadKeyID, dest string, progress func(int64, int64)) error {
	// URL contains the API key; do not log it.
	dlURL := fmt.Sprintf("%s/api/1/%s/upload/%s/download?download_key_id=%s",
		c.base, apiKey, uploadID, downloadKeyID)
	logger.Debug("auth: resolving CDN for upload id=%s", uploadID)

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
