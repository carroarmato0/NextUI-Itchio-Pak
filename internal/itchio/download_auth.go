package itchio

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

const apiItchIO = "https://api.itch.io"

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
//  2c. neither present (bundle key) → paginate profile/owned-keys to find downloads_url/key
//  2d. paginated scan also absent  → GET api.itch.io/games/GAME_ID/uploads (Bearer, no key needed)
//  2e. butler Bearer path fails    → GET itch.io/api/1/KEY/game/GAME_ID/uploads?download_key_id=ID
func (c *Client) FetchAuthUploads(apiKey, gameID, gameURL string) ([]Upload, string, error) {
	// Step 1: get download key info from the butler-style API.
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

	// 2a. Prefer downloads_url: pre-signed page URL that works for any purchase type.
	if key.DownloadsURL != "" {
		logger.Debug("auth: using signed-page path via downloads_url")
		return c.fetchAuthUploadsViaSignedPage(key.DownloadsURL)
	}

	// 2b. If the key string is present and we have the game URL, construct the
	// signed page URL ourselves. This handles bundle purchases where downloads_url
	// is absent but the key string is available.
	if key.Key != "" && gameURL != "" {
		signedURL := strings.TrimRight(gameURL, "/") + "/download/" + url.PathEscape(key.Key)
		logger.Debug("auth: constructing signed URL from key string + game URL")
		return c.fetchAuthUploadsViaSignedPage(signedURL)
	}

	if key.ID == 0 {
		logger.Warn("auth: owned-keys returned id=0, no downloads_url, no key string")
		return nil, "", fmt.Errorf("game not owned or API key invalid (no valid download key)")
	}

	// 2c. Fallback: paginate through all owned keys (no game_id filter) to find
	// downloads_url or key string for this game. The game_id-filtered endpoint omits
	// these fields for bundle purchases, but the paginated endpoint includes them.
	dlURL, keyStr := c.findDownloadsURLForGame(apiKey, gameID)
	if dlURL != "" {
		logger.Debug("auth: found downloads_url via paginated scan for game_id=%s", gameID)
		return c.fetchAuthUploadsViaSignedPage(dlURL)
	}
	if keyStr != "" && gameURL != "" {
		signedURL := strings.TrimRight(gameURL, "/") + "/download/" + url.PathEscape(keyStr)
		logger.Debug("auth: found key string via paginated scan, constructing signed URL")
		return c.fetchAuthUploadsViaSignedPage(signedURL)
	}

	// 2d. Try the butler API with Bearer auth. This does not require a download key —
	// it relies solely on the authenticated account owning the game. Works for bundle
	// purchases where itch.io issues no per-game download key.
	logger.Debug("auth: trying butler Bearer path for game_id=%s", gameID)
	butlerUploads, butlerErr := c.fetchAuthUploadsViaButler(apiKey, gameID)
	if butlerErr == nil && len(butlerUploads) > 0 {
		logger.Info("auth: butler Bearer path succeeded for game_id=%s (%d uploads)", gameID, len(butlerUploads))
		return butlerUploads, "bearer", nil
	}
	if butlerErr != nil {
		logger.Warn("auth: butler Bearer path failed: %v", butlerErr)
	} else {
		logger.Warn("auth: butler Bearer path returned no uploads for game_id=%s", gameID)
	}

	downloadKeyID := fmt.Sprintf("%d", key.ID)
	// downloadKeyID not logged — it ties the request to the user's account.

	// 2e. Last resort: list uploads via the simple API. The simple API (itch.io/api/1)
	// accepts both direct-purchase keys and bundle keys. The URL contains the API
	// key; do not log it.
	uploadsURL := fmt.Sprintf("%s/api/1/%s/game/%s/uploads?download_key_id=%s",
		c.base, apiKey, gameID, downloadKeyID)
	logger.Debug("auth: fetching upload list for game_id=%s", gameID)

	resp2, err := c.http.Get(uploadsURL)
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
	if resp2.StatusCode == http.StatusInternalServerError {
		logger.Warn("auth: upload list HTTP 500 (body: %.200s)", rawBody)
		return nil, "", fmt.Errorf("Game not owned or download key invalid — purchase this game to download it")
	}
	if resp2.StatusCode != http.StatusOK {
		logger.Error("auth: upload list HTTP %d: %.200s", resp2.StatusCode, rawBody)
		return nil, "", fmt.Errorf("fetch uploads: HTTP %d", resp2.StatusCode)
	}

	// itch.io returns {"uploads":[...]} when accessible, and {"uploads":{}} (an
	// object, not an array) when the game has no uploads or access is denied.
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

// findDownloadsURLForGame paginates through profile/owned-keys (no game_id filter)
// looking for the entry matching gameID and returns its downloads_url and key string.
// The paginated endpoint includes fields that the game_id-filtered endpoint may omit
// for bundle purchases.
func (c *Client) findDownloadsURLForGame(apiKey, gameID string) (downloadsURL, keyStr string) {
	targetID, err := strconv.ParseInt(gameID, 10, 64)
	if err != nil {
		logger.Warn("auth: paginated scan: invalid game_id %q: %v", gameID, err)
		return
	}
	logger.Debug("auth: paginated scan: searching for game_id=%s across owned-keys pages", gameID)
	for page := 1; page <= 20; page++ {
		req, err := http.NewRequest("GET",
			fmt.Sprintf("%s/profile/owned-keys?page=%d", c.butler, page), nil)
		if err != nil {
			logger.Warn("auth: paginated scan: build request page %d: %v", page, err)
			return
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		resp, err := c.http.Do(req)
		if err != nil {
			logger.Warn("auth: paginated scan: page %d request error: %v", page, err)
			return
		}
		var envelope struct {
			OwnedKeys json.RawMessage `json:"owned_keys"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&envelope)
		resp.Body.Close()
		if decodeErr != nil {
			logger.Warn("auth: paginated scan: decode page %d: %v", page, decodeErr)
			return
		}
		if len(envelope.OwnedKeys) == 0 || envelope.OwnedKeys[0] != '[' {
			logger.Debug("auth: paginated scan: page %d returned non-array, stopping", page)
			return
		}
		var items []struct {
			DownloadsURL string `json:"downloads_url"`
			Key          string `json:"key"`
			Game         struct {
				ID int64 `json:"id"`
			} `json:"game"`
		}
		if err := json.Unmarshal(envelope.OwnedKeys, &items); err != nil {
			logger.Warn("auth: paginated scan: unmarshal page %d: %v", page, err)
			return
		}
		if len(items) == 0 {
			logger.Debug("auth: paginated scan: page %d empty, stopping", page)
			return
		}
		for _, item := range items {
			if item.Game.ID == targetID {
				logger.Debug("auth: paginated scan: found game_id=%s on page %d: downloads_url=%s key_str=%s",
					gameID, page, presentAbsent(item.DownloadsURL), presentAbsent(item.Key))
				return item.DownloadsURL, item.Key
			}
		}
		logger.Debug("auth: paginated scan: page %d scanned (%d entries), not found yet", page, len(items))
	}
	logger.Warn("auth: paginated scan: game_id=%s not found in owned-keys pages", gameID)
	return
}

// fetchAuthUploadsViaButler lists uploads for a game using the butler API with Bearer
// auth. This works when the user owns the game via a bundle and itch.io issues no
// per-game download key — account ownership is enough.
func (c *Client) fetchAuthUploadsViaButler(apiKey, gameID string) ([]Upload, error) {
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/games/%s/uploads", c.butler, gameID), nil)
	if err != nil {
		return nil, fmt.Errorf("build butler uploads request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	logger.Debug("auth: butler: GET /games/%s/uploads", gameID)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("butler uploads request: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("butler uploads read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		logger.Warn("auth: butler uploads HTTP %d: %.200s", resp.StatusCode, rawBody)
		return nil, fmt.Errorf("butler uploads: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Uploads []struct {
			ID       int64  `json:"id"`
			Filename string `json:"filename"`
		} `json:"uploads"`
	}
	if err := json.Unmarshal(rawBody, &result); err != nil {
		logger.Error("auth: butler uploads decode: %v (body: %.200s)", err, rawBody)
		return nil, fmt.Errorf("butler uploads decode: %w", err)
	}

	var uploads []Upload
	for _, u := range result.Uploads {
		ext := strings.ToLower(filepath.Ext(u.Filename))
		uploadIDStr := fmt.Sprintf("%d", u.ID)
		if ext == ".gb" || ext == ".gbc" || ext == ".zip" {
			uploads = append(uploads, Upload{
				Filename: u.Filename,
				UploadID: uploadIDStr,
			})
			logger.Debug("auth: butler: found ROM %s id=%d", u.Filename, u.ID)
		} else if !isSkippableExt(ext) {
			uploads = append(uploads, Upload{
				Filename:    u.Filename,
				UploadID:    uploadIDStr,
				NeedsFormat: true,
			})
			logger.Debug("auth: butler: found unknown-format %s id=%d", u.Filename, u.ID)
		} else {
			logger.Debug("auth: butler: skipping %s (ext=%q)", u.Filename, ext)
		}
	}

	knownCount := 0
	for _, u := range uploads {
		if !u.NeedsFormat {
			knownCount++
		}
	}
	logger.Debug("auth: butler: %d known ROM(s), %d unknown-format, %d total uploads",
		knownCount, len(uploads)-knownCount, len(result.Uploads))
	return uploads, nil
}

// DownloadAuthBearer resolves the CDN URL for an upload using the butler API with
// Bearer auth and streams it to dest. Used when the game was acquired via a bundle
// and no per-game download key is available.
func (c *Client) DownloadAuthBearer(apiKey, uploadID, dest string, progress func(int64, int64)) error {
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/uploads/%s/download", c.butler, uploadID), nil)
	if err != nil {
		return fmt.Errorf("build bearer download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	logger.Debug("auth: bearer: resolving CDN for upload id=%s", uploadID)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("bearer download resolve: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error("auth: bearer download HTTP %d", resp.StatusCode)
		return fmt.Errorf("bearer download resolve: HTTP %d", resp.StatusCode)
	}

	var result struct {
		URL    string   `json:"url"`
		Errors []string `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("bearer download decode: %w", err)
	}
	if len(result.Errors) > 0 {
		logger.Error("auth: bearer download error: %s", strings.Join(result.Errors, "; "))
		return fmt.Errorf("bearer download error: %s", strings.Join(result.Errors, "; "))
	}
	if result.URL == "" {
		logger.Error("auth: bearer download: empty CDN URL")
		return fmt.Errorf("bearer download: empty CDN URL")
	}

	logger.Info("auth: bearer: streaming to %s", dest)
	return c.streamToFile(result.URL, dest, progress)
}

// DownloadAuthUpload resolves the CDN URL for an owned upload and streams it to dest.
// Uses the simple API (itch.io/api/1) which accepts both direct-purchase and bundle keys.
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
