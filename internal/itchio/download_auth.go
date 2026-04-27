package itchio

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// OwnedKey represents one purchase granting download access to a game.
// A game may appear multiple times (once per purchase transaction) — e.g.
// once for an individual purchase and once from a bundle.
type OwnedKey struct {
	ID         int64     // numeric download key ID — pass as download_key_id
	PurchaseID int64     // ties this key to a specific purchase transaction
	CreatedAt  time.Time // when this purchase was made
	Downloads  int       // how many times this key has been used to download
	// BundleSize is the number of distinct games in the same purchase.
	// 1 = individual purchase; >1 = bundle purchase.
	BundleSize int
	// BundleName is the human-readable bundle name, populated by AnnotateBundleNames.
	// Empty for individual purchases.
	BundleName string
}

// rawOwnedKey is one entry from the API before enrichment.
type rawOwnedKey struct {
	ID         int64
	GameID     int64
	PurchaseID int64
	Downloads  int
	CreatedAt  string
}

// FetchOwnedKeys returns every purchase key the user holds for the given game.
// It paginates through all pages of api.itch.io/profile/owned-keys (the
// game_id query parameter is ignored server-side, so filtering is done here).
//
// BundleSize on each returned key reflects how many distinct games share the
// same purchase transaction: 1 = individual purchase; >1 = bundle purchase.
//
// Returns a non-empty slice when the game is owned, or an error when it is
// not owned / the API key is invalid.
func (c *Client) FetchOwnedKeys(apiKey, gameID string) ([]OwnedKey, error) {
	targetID, err := strconv.ParseInt(gameID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid game_id %q: %w", gameID, err)
	}

	// Collect every key across all pages so we can compute bundle sizes.
	var all []rawOwnedKey
	const maxPages = 20
	for page := 1; page <= maxPages; page++ {
		keysURL := fmt.Sprintf("%s/profile/owned-keys?page=%d", c.butler, page)
		req, err := http.NewRequest("GET", keysURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build owned-keys request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch owned keys (page %d): %w", page, err)
		}

		// itch.io returns {"owned_keys":{}} (object, not array) on the last page.
		// Use RawMessage so we can detect this before attempting slice decode.
		var envelope struct {
			PerPage   int             `json:"per_page"`
			OwnedKeys json.RawMessage `json:"owned_keys"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&envelope)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			logger.Error("auth: owned-keys HTTP %d (page %d)", resp.StatusCode, page)
			return nil, fmt.Errorf("fetch owned keys: HTTP %d", resp.StatusCode)
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("decode owned keys (page %d): %w", page, decodeErr)
		}

		if len(envelope.OwnedKeys) == 0 || envelope.OwnedKeys[0] != '[' {
			break // empty object {} — no more pages
		}

		var keyItems []struct {
			ID         int64  `json:"id"`
			GameID     int64  `json:"game_id"`
			PurchaseID int64  `json:"purchase_id"`
			Downloads  int    `json:"downloads"`
			CreatedAt  string `json:"created_at"`
		}
		if err := json.Unmarshal(envelope.OwnedKeys, &keyItems); err != nil {
			return nil, fmt.Errorf("decode owned keys (page %d): %w", page, err)
		}

		logger.Debug("auth: owned-keys page %d — %d entries", page, len(keyItems))
		for _, k := range keyItems {
			all = append(all, rawOwnedKey{
				ID: k.ID, GameID: k.GameID, PurchaseID: k.PurchaseID,
				Downloads: k.Downloads, CreatedAt: k.CreatedAt,
			})
		}

		if len(keyItems) < envelope.PerPage || envelope.PerPage == 0 {
			break // last page
		}
	}

	// Count distinct games per purchase_id to identify bundles vs. individual purchases.
	purchaseGameCounts := map[int64]int{}
	for _, k := range all {
		purchaseGameCounts[k.PurchaseID]++
	}

	var matches []OwnedKey
	for _, k := range all {
		if k.GameID != targetID {
			continue
		}
		t, _ := time.Parse(time.RFC3339, k.CreatedAt)
		matches = append(matches, OwnedKey{
			ID:         k.ID,
			PurchaseID: k.PurchaseID,
			CreatedAt:  t,
			Downloads:  k.Downloads,
			BundleSize: purchaseGameCounts[k.PurchaseID],
		})
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("Game not owned or API key invalid (game_id=%s not found in owned keys)", gameID)
	}
	logger.Debug("auth: found %d owned key(s) for game_id=%s", len(matches), gameID)
	return matches, nil
}

// AnnotateBundleNames sets BundleName on bundle keys (BundleSize > 1) by
// matching them — sorted by CreatedAt ascending — to bundleNames (in page order).
// Individual keys are skipped. If there are more bundle keys than names, the
// excess keys keep an empty BundleName (displayed as "Bundle purchase" fallback).
func AnnotateBundleNames(keys []OwnedKey, bundleNames []string) []OwnedKey {
	if len(bundleNames) == 0 {
		return keys
	}
	// Collect indices of bundle keys in ascending CreatedAt order.
	type indexedKey struct {
		idx int
		t   time.Time
	}
	var bundleIdxs []indexedKey
	for i, k := range keys {
		if k.BundleSize > 1 {
			bundleIdxs = append(bundleIdxs, indexedKey{i, k.CreatedAt})
		}
	}
	// Sort by CreatedAt ascending so we assign names in the same order they
	// appear on the public game page (oldest bundle first).
	for i := 1; i < len(bundleIdxs); i++ {
		for j := i; j > 0 && bundleIdxs[j].t.Before(bundleIdxs[j-1].t); j-- {
			bundleIdxs[j], bundleIdxs[j-1] = bundleIdxs[j-1], bundleIdxs[j]
		}
	}
	for nameIdx, bi := range bundleIdxs {
		if nameIdx >= len(bundleNames) {
			break
		}
		keys[bi.idx].BundleName = bundleNames[nameIdx]
	}
	return keys
}

// FetchUploadsForKey lists the .gb/.gbc uploads available for a game the
// user owns, using one specific download key.
//
// It calls the simple itch.io API v1 which accepts the numeric owned-key ID
// as download_key_id — this works for both individual purchases and bundle
// purchases.
func (c *Client) FetchUploadsForKey(apiKey, gameID, downloadKeyID string) ([]Upload, error) {
	// downloadKeyID not logged — it identifies the user's purchase.
	uploadsURL := fmt.Sprintf("%s/api/1/%s/game/%s/uploads?download_key_id=%s",
		c.base, apiKey, gameID, downloadKeyID)
	logger.Debug("auth: fetching upload list for game_id=%s", gameID)

	resp, err := c.http.Get(uploadsURL)
	if err != nil {
		return nil, fmt.Errorf("fetch uploads: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		logger.Warn("auth: upload list HTTP %d — key may not grant access to this game", resp.StatusCode)
		return nil, fmt.Errorf("Game not owned or API key does not grant access to this game's downloads")
	}
	if resp.StatusCode != http.StatusOK {
		logger.Error("auth: upload list HTTP %d", resp.StatusCode)
		return nil, fmt.Errorf("fetch uploads: HTTP %d", resp.StatusCode)
	}

	// The simple API returns {"uploads":[...]} normally, but {"uploads":{}} (an
	// object) when there are no uploads or access is restricted. Decode raw so
	// both forms can be handled without panicking.
	var envelope struct {
		Uploads json.RawMessage `json:"uploads"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode uploads response: %w", err)
	}

	var items []struct {
		ID       int64  `json:"id"`
		Filename string `json:"filename"`
	}
	if len(envelope.Uploads) > 0 && envelope.Uploads[0] == '[' {
		if err := json.Unmarshal(envelope.Uploads, &items); err != nil {
			return nil, fmt.Errorf("decode uploads array: %w", err)
		}
	} else {
		logger.Debug("auth: uploads field is not an array (%.50s) — treating as empty", envelope.Uploads)
	}

	var uploads []Upload
	for _, u := range items {
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
			logger.Debug("auth: found unknown-format %s id=%d (user will choose)", u.Filename, u.ID)
		} else {
			logger.Debug("auth: skipping %s (ext=%q)", u.Filename, ext)
		}
	}

	known := 0
	for _, u := range uploads {
		if !u.NeedsFormat {
			known++
		}
	}
	logger.Debug("auth: %d known ROM(s), %d unknown-format from %d total uploads",
		known, len(uploads)-known, len(items))
	return uploads, nil
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
