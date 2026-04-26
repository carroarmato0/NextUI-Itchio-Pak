package itchio

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

const apiItchIO = "https://api.itch.io"

// FetchAuthUploadsViaGamePage lists uploads for an owned game by POSTing to the
// game's own download_url endpoint with the API key as a Bearer token. This
// mirrors what the itch.io website does when a logged-in user clicks Download,
// and works for both direct purchases and bundle purchases.
//
// Returns (nil, err) when the game is not owned or the API key is rejected.
func (c *Client) FetchAuthUploadsViaGamePage(apiKey, gameURL string) ([]Upload, error) {
	logger.Debug("auth: game-page path for %s", gameURL)

	// Step 1: GET game page to extract CSRF token.
	req, err := http.NewRequest("GET", gameURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build game page request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch game page: %w", err)
	}
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read game page: %w", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		logger.Error("auth: game page HTTP %d", resp.StatusCode)
		return nil, fmt.Errorf("fetch game page: HTTP %d", resp.StatusCode)
	}

	csrfM := csrfRegex.FindStringSubmatch(string(body))
	if len(csrfM) < 2 {
		return nil, fmt.Errorf("csrf_token not found on game page")
	}
	csrf := csrfM[1]
	logger.Debug("auth: game page CSRF %s", presentAbsent(csrf))

	// Step 2: POST to download_url with Bearer auth. itch.io checks the API key
	// and returns a signed URL if the user owns the game (regardless of purchase
	// method — direct or bundle).
	postURL := strings.TrimRight(gameURL, "/") + "/download_url"
	form := url.Values{"csrf_token": {csrf}}
	req2, err := http.NewRequest("POST", postURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build download_url request: %w", err)
	}
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.Header.Set("Authorization", "Bearer "+apiKey)

	postResp, err := c.http.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("download_url POST: %w", err)
	}
	defer postResp.Body.Close()

	var dlResult struct {
		URL    string   `json:"url"`
		Errors []string `json:"errors"`
	}
	if err := json.NewDecoder(postResp.Body).Decode(&dlResult); err != nil {
		return nil, fmt.Errorf("parse download_url response: %w", err)
	}
	if len(dlResult.Errors) > 0 {
		logger.Warn("auth: game-page download_url errors: %s", strings.Join(dlResult.Errors, "; "))
		return nil, fmt.Errorf("game not owned or API key invalid: %s", strings.Join(dlResult.Errors, "; "))
	}
	if dlResult.URL == "" {
		logger.Warn("auth: game-page download_url returned empty URL (game not owned or Bearer auth not accepted)")
		return nil, fmt.Errorf("game not owned or download requires purchase")
	}
	logger.Debug("auth: game-page signed URL received")

	// Step 3: extract the download key from the signed URL path.
	key := extractDownloadKey(dlResult.URL)
	if key == "" {
		return nil, fmt.Errorf("auth: could not extract download key from signed URL")
	}

	// Step 4: parse the signed download page for upload IDs + filenames + CSRF token.
	dlPage, err := c.ParseDownloadPage(dlResult.URL)
	if err != nil {
		return nil, fmt.Errorf("auth: parse signed download page: %w", err)
	}

	// Step 5: build resolver URLs. Same format as the free-game path.
	base := strings.TrimRight(gameURL, "/")
	var uploads []Upload
	for _, u := range dlPage.Uploads {
		resolverURL := base + "/file/" + u.UploadID +
			"?key=" + url.QueryEscape(key) +
			"&csrf=" + url.QueryEscape(dlPage.CSRFToken)
		logger.Debug("auth: found %s id=%s (game-page path)", u.Filename, u.UploadID)
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
	logger.Debug("auth: %d known ROM(s), %d unknown-format file(s) (game-page path)",
		knownCount, len(uploads)-knownCount)
	if len(uploads) == 0 {
		logger.Warn("auth: no downloadable uploads found via game-page path")
	}
	return uploads, nil
}

// FetchAuthUploads lists .gb/.gbc uploads for a paid game the user owns.
//
// Flow:
//  1. GET api.itch.io/profile/owned-keys?game_id=GAME_ID  → downloads_url (preferred)
//     or numeric download key ID (fallback)
//  2a. If downloads_url present: parse the signed download page (works for direct
//      purchases AND bundle purchases). Returns uploads with URL set, empty keyID.
//  2b. Otherwise: GET api.itch.io/games/GAME_ID/uploads?download_key_id=KEY_ID
func (c *Client) FetchAuthUploads(apiKey, gameID string) ([]Upload, string, error) {
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
			DownloadsURL string `json:"downloads_url"`
		} `json:"owned_keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&keysResult); err != nil {
		return nil, "", fmt.Errorf("decode owned keys: %w", err)
	}
	if len(keysResult.OwnedKeys) == 0 {
		return nil, "", fmt.Errorf("game not owned or API key invalid (no download key found)")
	}

	key := keysResult.OwnedKeys[0]

	// Prefer downloads_url: it is a signed page URL generated by itch.io that
	// works for both direct purchases and bundle purchases. The numeric key only
	// works for direct purchases — bundle keys cause HTTP 500 on the uploads endpoint.
	if key.DownloadsURL != "" {
		logger.Debug("auth: using signed-page path via downloads_url")
		return c.fetchAuthUploadsViaSignedPage(key.DownloadsURL)
	}

	// Fall back to butler API with numeric download key.
	if key.ID == 0 {
		logger.Warn("auth: owned-keys returned id=0 and no downloads_url (game may not be owned)")
		return nil, "", fmt.Errorf("game not owned or API key invalid (no valid download key)")
	}
	downloadKeyID := fmt.Sprintf("%d", key.ID)
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

// fetchAuthUploadsViaSignedPage uses the signed downloads_url from the owned-keys
// response to list uploads. This path works for both direct purchases and bundle
// purchases; the butler numeric key does not work for bundle keys.
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
	// Empty keyID signals to callers that DownloadFree (not DownloadAuthUpload) handles these uploads.
	return uploads, "", nil
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
