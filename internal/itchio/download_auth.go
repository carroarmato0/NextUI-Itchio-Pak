package itchio

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
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
//
// It asks api.itch.io/profile/owned-keys with game_id, which itch.io added
// for this app. Until that filter is applied server-side the endpoint still
// returns the whole library (verified 2026-09-24), so the result is filtered
// here as well and both shapes work.
//
// BundleSize on each returned key reflects how many distinct games share the
// same purchase transaction: 1 = individual purchase; >1 = bundle purchase.
// That needs the whole library, so a filtered answer takes the counts from
// the last full scan (ValidateAPIKey at startup), or scans again without it.
//
// Returns a non-empty slice when the game is owned, or an error when it is
// not owned / the API key is invalid.
func (c *Client) FetchOwnedKeys(apiKey, gameID string) ([]OwnedKey, error) {
	targetID, err := strconv.ParseInt(gameID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid game_id %q: %w", gameID, err)
	}

	all, err := c.scanOwnedKeys(apiKey, gameID)
	if err != nil {
		return nil, err
	}

	filtered := len(all) > 0
	for _, k := range all {
		if k.GameID != targetID {
			filtered = false
			break
		}
	}
	var counts map[int64]int
	if filtered {
		counts = c.cachedPurchaseCounts(all)
		if counts == nil {
			logger.Debug("auth: owned-keys filtered by game_id; no cached bundle sizes, scanning library")
			full, err := c.scanOwnedKeys(apiKey, "")
			if err != nil {
				return nil, err
			}
			counts = purchaseGameCounts(full)
			c.setPurchaseCounts(counts)
		}
	} else {
		counts = purchaseGameCounts(all)
		c.setPurchaseCounts(counts)
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
			BundleSize: counts[k.PurchaseID],
		})
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("Game not owned or API key invalid (game_id=%s not found in owned keys)", gameID)
	}
	logger.Debug("auth: found %d owned key(s) for game_id=%s (server-filtered=%v)", len(matches), gameID, filtered)
	return matches, nil
}

// scanOwnedKeys pages through api.itch.io/profile/owned-keys, optionally
// passing game_id, and returns every entry.
func (c *Client) scanOwnedKeys(apiKey, gameID string) ([]rawOwnedKey, error) {
	var all []rawOwnedKey
	const maxPages = 20
	for page := 1; page <= maxPages; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}}
		if gameID != "" {
			q.Set("game_id", gameID)
		}
		req, err := http.NewRequest("GET", c.butler+"/profile/owned-keys?"+q.Encode(), nil)
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
	return all, nil
}

// purchaseGameCounts counts distinct games per purchase_id, which tells
// bundles (>1) from individual purchases (1).
func purchaseGameCounts(keys []rawOwnedKey) map[int64]int {
	games := map[int64]map[int64]bool{}
	for _, k := range keys {
		if games[k.PurchaseID] == nil {
			games[k.PurchaseID] = map[int64]bool{}
		}
		games[k.PurchaseID][k.GameID] = true
	}
	counts := make(map[int64]int, len(games))
	for pid, g := range games {
		counts[pid] = len(g)
	}
	return counts
}

func (c *Client) setPurchaseCounts(counts map[int64]int) {
	c.ownedMu.Lock()
	c.purchaseCounts = counts
	c.ownedMu.Unlock()
}

// cachedPurchaseCounts returns the cached counts if they cover every
// purchase in keys, else nil.
func (c *Client) cachedPurchaseCounts(keys []rawOwnedKey) map[int64]int {
	c.ownedMu.Lock()
	defer c.ownedMu.Unlock()
	if c.purchaseCounts == nil {
		return nil
	}
	for _, k := range keys {
		if _, ok := c.purchaseCounts[k.PurchaseID]; !ok {
			return nil
		}
	}
	return c.purchaseCounts
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
	sort.Slice(bundleIdxs, func(i, j int) bool {
		return bundleIdxs[i].t.Before(bundleIdxs[j].t)
	})
	for nameIdx, bi := range bundleIdxs {
		if nameIdx >= len(bundleNames) {
			break
		}
		keys[bi.idx].BundleName = bundleNames[nameIdx]
	}
	return keys
}

// FetchUploadsForKey lists the ROM uploads of a game through the itch.io API
// (api.itch.io/games/{id}/uploads), sending the key in the Authorization
// header. downloadKeyID selects the purchase that grants access; pass "" for
// a free or name-your-own-price game, which the API lists without one.
func (c *Client) FetchUploadsForKey(apiKey, gameID, downloadKeyID string) ([]Upload, error) {
	q := url.Values{}
	if downloadKeyID != "" {
		q.Set("download_key_id", downloadKeyID)
	}
	uploadsURL := fmt.Sprintf("%s/games/%s/uploads", c.butler, url.PathEscape(gameID))
	if len(q) > 0 {
		uploadsURL += "?" + q.Encode()
	}
	// downloadKeyID not logged — it identifies the user's purchase.
	logger.Debug("auth: fetching upload list for game_id=%s (with key=%v)", gameID, downloadKeyID != "")

	req, err := http.NewRequest("GET", uploadsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build uploads request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
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

	// {"uploads":[...]} normally, but {"uploads":{}} (an object) when there
	// are none, and {"errors":[...]} on failure. Decode raw so every form is
	// handled without a decode error hiding the others.
	var envelope struct {
		Uploads json.RawMessage `json:"uploads"`
		Errors  []string        `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode uploads response: %w", err)
	}
	if len(envelope.Errors) > 0 {
		logger.Error("auth: upload list error: %s", strings.Join(envelope.Errors, "; "))
		return nil, fmt.Errorf("fetch uploads: %s", strings.Join(envelope.Errors, "; "))
	}

	// "traits" is deliberately not decoded: it is {} when empty and an array
	// otherwise, the same object-vs-array quirk as owned-keys.
	var items []struct {
		ID        int64     `json:"id"`
		Filename  string    `json:"filename"`
		Size      int64     `json:"size"`
		MD5       string    `json:"md5_hash"`
		BuildID   int64     `json:"build_id"`
		UpdatedAt time.Time `json:"updated_at"`
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
		up := Upload{
			Filename:  u.Filename,
			UploadID:  strconv.FormatInt(u.ID, 10),
			Size:      u.Size,
			MD5:       u.MD5,
			BuildID:   u.BuildID,
			UpdatedAt: u.UpdatedAt,
		}
		ext := strings.ToLower(filepath.Ext(u.Filename))
		if ext == ".gb" || ext == ".gbc" || ext == ".gba" || ext == ".nes" || ext == ".md" || ext == ".gen" || ext == ".smd" || ext == ".zip" {
			uploads = append(uploads, up)
			logger.Debug("auth: found ROM %s id=%d size=%d", u.Filename, u.ID, u.Size)
		} else if !isSkippableExt(ext) {
			up.NeedsFormat = true
			uploads = append(uploads, up)
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

// CreateDownloadSession opens a download session for one install
// (POST api.itch.io/games/{id}/download-sessions) and returns its UUID. Every
// request of that install passes the UUID, so itch.io counts range reads and
// the download itself as one download.
func (c *Client) CreateDownloadSession(apiKey, gameID, downloadKeyID string) (string, error) {
	form := url.Values{}
	if downloadKeyID != "" {
		form.Set("download_key_id", downloadKeyID)
	}
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/games/%s/download-sessions", c.butler, url.PathEscape(gameID)),
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build download-session request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("create download session: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		UUID   string   `json:"uuid"`
		Errors []string `json:"errors"`
	}
	decodeErr := json.NewDecoder(resp.Body).Decode(&result)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		logger.Warn("auth: download session HTTP %d for game_id=%s %s", resp.StatusCode, gameID, strings.Join(result.Errors, "; "))
		return "", fmt.Errorf("create download session: HTTP %d", resp.StatusCode)
	}
	if decodeErr != nil {
		return "", fmt.Errorf("decode download session: %w", decodeErr)
	}
	if result.UUID == "" {
		return "", fmt.Errorf("create download session: %s", strings.Join(append(result.Errors, "no uuid in response"), "; "))
	}
	logger.Info("auth: download session opened for game_id=%s", gameID)
	return result.UUID, nil
}

// ResolveAuthURL resolves the signed CDN URL for an upload fetched through the
// API (GET api.itch.io/uploads/{id}/download). The endpoint answers with a
// redirect to the CDN; it is not followed, so the caller gets the URL first
// for ZIP inspection or the magic-byte probe, and the Authorization header
// never reaches the CDN.
func (c *Client) ResolveAuthURL(apiKey, uploadID string, session *roms.DownloadSession) (string, error) {
	if session == nil {
		return "", fmt.Errorf("resolve upload %s: no download session", uploadID)
	}
	q := url.Values{}
	if session.KeyID != "" {
		q.Set("download_key_id", session.KeyID)
	}
	uuid, err := session.UUID(func() (string, error) {
		return c.CreateDownloadSession(apiKey, session.GameID, session.KeyID)
	})
	if err != nil {
		// Not fatal: the download works without a session, it just is not
		// grouped with the other requests of this install.
		logger.Warn("auth: continuing without a download session: %v", err)
	}
	if uuid != "" {
		q.Set("uuid", uuid)
	}
	dlURL := fmt.Sprintf("%s/uploads/%s/download", c.butler, url.PathEscape(uploadID))
	if len(q) > 0 {
		dlURL += "?" + q.Encode()
	}
	logger.Debug("auth: resolving CDN for upload id=%s (session=%v)", uploadID, uuid != "")

	req, err := http.NewRequest("GET", dlURL, nil)
	if err != nil {
		return "", fmt.Errorf("build resolve request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	noFollow := &http.Client{
		Transport: c.http.Transport,
		Jar:       c.http.Jar,
		Timeout:   c.http.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := noFollow.Do(req)
	if err != nil {
		return "", fmt.Errorf("resolve auth CDN URL: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		loc, err := resp.Location()
		if err != nil {
			logger.Error("auth: CDN redirect HTTP %d without a usable Location", resp.StatusCode)
			return "", fmt.Errorf("auth CDN redirect without Location: %w", err)
		}
		// CDN URL contains signed tokens — do not log it.
		return loc.String(), nil
	case resp.StatusCode == http.StatusOK && strings.Contains(resp.Header.Get("Content-Type"), "json"):
		// Some deployments answer with {"url": ...} instead of redirecting.
		var result struct {
			URL    string   `json:"url"`
			Errors []string `json:"errors"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", fmt.Errorf("decode auth CDN response: %w", err)
		}
		if len(result.Errors) > 0 {
			logger.Error("auth: CDN error: %s", strings.Join(result.Errors, "; "))
			return "", fmt.Errorf("auth CDN error: %s", strings.Join(result.Errors, "; "))
		}
		if result.URL == "" {
			logger.Error("auth: empty CDN URL from resolver")
			return "", fmt.Errorf("empty CDN URL from auth resolver")
		}
		return result.URL, nil
	default:
		var result struct {
			Errors []string `json:"errors"`
		}
		json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&result)
		logger.Error("auth: CDN resolve HTTP %d %s", resp.StatusCode, strings.Join(result.Errors, "; "))
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
			return "", fmt.Errorf("Game not owned or API key does not grant access to this download")
		}
		return "", fmt.Errorf("auth CDN resolve status %d", resp.StatusCode)
	}
}

// DownloadAuthUpload resolves the CDN URL for an API upload and streams it to dest.
func (c *Client) DownloadAuthUpload(apiKey, uploadID string, session *roms.DownloadSession, dest string, progress func(int64, int64)) error {
	cdnURL, err := c.ResolveAuthURL(apiKey, uploadID, session)
	if err != nil {
		return err
	}
	logger.Info("auth: streaming to %s", dest)
	return c.streamToFile(cdnURL, dest, progress)
}
