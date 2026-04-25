package itchio

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// knownNonROMExts lists extensions that are definitely not GB/GBC ROM files.
// Uploads with these extensions are silently dropped when scanning a game's
// upload list. Anything not in this map (including no extension, version-number
// suffixes like ".0", and ".zip") is returned with NeedsFormat=true so the
// user can classify it manually.
var knownNonROMExts = map[string]bool{
	".7z": true, ".tar": true, ".gz": true, ".rar": true, ".bz2": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true, ".webp": true,
	".mp3": true, ".ogg": true, ".wav": true, ".flac": true, ".aac": true,
	".pdf": true, ".txt": true, ".md": true, ".epub": true, ".mobi": true,
	".mp4": true, ".avi": true, ".mkv": true, ".mov": true,
	".exe": true, ".dmg": true, ".apk": true,
	".pocket": true, ".nes": true, ".gba": true, ".nds": true, ".sfc": true, ".smc": true,
}

func isSkippableExt(ext string) bool {
	return knownNonROMExts[strings.ToLower(ext)]
}

// presentAbsent returns "present" when s is non-empty, "absent" otherwise.
// Used to log whether a token exists without logging its value.
func presentAbsent(s string) string {
	if s != "" {
		return "present"
	}
	return "absent"
}

// FetchUploads returns the list of .gb/.gbc files available for free download.
//
// Flow:
//  1. GET game page → CSRF token
//  2. POST gameURL/download_url → signed download page URL containing the key
//  3. GET signed page → parse upload IDs + filenames via ParseDownloadPage
//  4. Construct a resolver URL for each upload: gameURL/file/UPLOAD_ID?key=KEY
//
// The resolver URL is stored as Upload.URL. Pass it to DownloadFree to resolve
// the actual CDN link and stream the file.
func (c *Client) FetchUploads(gameURL string) ([]Upload, error) {
	// Step 1: get CSRF token from game page
	resp, err := c.http.Get(gameURL)
	if err != nil {
		return nil, fmt.Errorf("fetch game page: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read game page: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		logger.Error("uploads: game page HTTP %d", resp.StatusCode)
		return nil, fmt.Errorf("fetch game page: HTTP %d", resp.StatusCode)
	}

	csrfM := csrfRegex.FindStringSubmatch(string(body))
	if len(csrfM) < 2 {
		return nil, fmt.Errorf("csrf_token not found on game page")
	}
	csrf := csrfM[1]

	// Step 2: POST to get the signed download page URL
	postURL := strings.TrimRight(gameURL, "/") + "/download_url"
	form := url.Values{"csrf_token": {csrf}, "suggested_amount": {"0"}}
	postResp, err := c.http.Post(postURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("download_url POST: %w", err)
	}
	defer postResp.Body.Close()

	var dlResult struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(postResp.Body).Decode(&dlResult); err != nil {
		return nil, fmt.Errorf("parse download_url response: %w", err)
	}
	if dlResult.URL == "" {
		return nil, fmt.Errorf("download_url returned empty url (game may be paid or require login)")
	}
	// The signed URL contains a download key — do not log it.
	logger.Debug("uploads: signed download URL received")

	// Step 3: extract the download key from the signed URL path
	// Format: https://author.itch.io/game/download/KEY
	key := extractDownloadKey(dlResult.URL)
	if key == "" {
		return nil, fmt.Errorf("could not extract download key from signed URL")
	}
	// The key value is sensitive — do not log it.
	logger.Debug("uploads: download key extracted")

	// Step 4: parse the signed download page for upload IDs + filenames + CSRF token
	dlPage, err := c.ParseDownloadPage(dlResult.URL)
	if err != nil {
		return nil, fmt.Errorf("parse download page: %w", err)
	}

	// Step 5: construct resolver URLs for each .gb/.gbc upload.
	// The resolver URL embeds both the download key and the signed-page CSRF token
	// so that DownloadFree can include both in its POST body.
	base := strings.TrimRight(gameURL, "/")
	var uploads []Upload
	for _, u := range dlPage.Uploads {
		resolverURL := base + "/file/" + u.UploadID +
			"?key=" + url.QueryEscape(key) +
			"&csrf=" + url.QueryEscape(dlPage.CSRFToken)
		logger.Debug("uploads: found %s id=%s", u.Filename, u.UploadID)
		uploads = append(uploads, Upload{
			Filename: u.Filename,
			UploadID: u.UploadID,
			URL:      resolverURL,
		})
	}
	return uploads, nil
}

// extractDownloadKey pulls the last path segment from a signed download URL.
// e.g. "https://author.itch.io/game/download/ABCDEF" → "ABCDEF"
//
// Uses EscapedPath (not Path) so that %2F-encoded slashes within the key are
// not treated as path separators before the final segment is URL-decoded.
func extractDownloadKey(signedURL string) string {
	parsed, err := url.Parse(signedURL)
	if err != nil {
		return ""
	}
	rawPath := parsed.EscapedPath()
	parts := strings.Split(strings.Trim(rawPath, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	key, err := url.PathUnescape(parts[len(parts)-1])
	if err != nil {
		return parts[len(parts)-1]
	}
	return key
}

// extractKeyID parses the itch.io download key JWT and returns the numeric
// download key ID embedded in its payload.
func extractKeyID(jwtKey string) string {
	dotIdx := strings.Index(jwtKey, ".")
	if dotIdx < 0 {
		return ""
	}
	payload := jwtKey[:dotIdx]
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(payload)
		if err != nil {
			return ""
		}
	}
	var p struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &p); err != nil || p.ID == 0 {
		return ""
	}
	return fmt.Sprintf("%d", p.ID)
}

// DownloadFree resolves the CDN URL for a free game upload and streams it.
//
// upload.URL must be a resolver endpoint of the form:
//
//	gameURL/file/UPLOAD_ID?key=KEY&csrf=CSRF
func (c *Client) DownloadFree(_ string, upload Upload, dest string, progress func(int64, int64)) error {
	// Parse the resolver URL to extract base path, key, and csrf.
	parsed, err := url.Parse(upload.URL)
	if err != nil {
		return fmt.Errorf("parse resolver URL: %w", err)
	}
	key := parsed.Query().Get("key")
	csrf := parsed.Query().Get("csrf")

	keyID := extractKeyID(key)
	baseURL := parsed.Scheme + "://" + parsed.Host + parsed.Path
	// Log token presence only — never log CSRF or key values.
	logger.Debug("uploads: POST resolver csrf=%s key=%s", presentAbsent(csrf), presentAbsent(key))

	form := url.Values{"csrf_token": {csrf}, "download_key_id": {keyID}}
	resp, err := c.http.Post(baseURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("resolve CDN URL: %w", err)
	}
	defer resp.Body.Close()

	rawBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("read resolver response: %w", readErr)
	}

	if resp.StatusCode != http.StatusOK {
		logger.Error("uploads: resolver HTTP %d: %.200s", resp.StatusCode, rawBody)
		return fmt.Errorf("resolve CDN URL: HTTP %d: %.200s", resp.StatusCode, rawBody)
	}

	var result struct {
		URL    string   `json:"url"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(rawBody, &result); err != nil {
		logger.Error("uploads: parse resolver response: %v (body: %.200s)", err, rawBody)
		return fmt.Errorf("parse CDN URL response: %w (body: %.200s)", err, rawBody)
	}
	if len(result.Errors) > 0 {
		logger.Error("uploads: resolver error: %s", strings.Join(result.Errors, "; "))
		return fmt.Errorf("resolver error: %s", strings.Join(result.Errors, "; "))
	}
	if result.URL == "" {
		logger.Error("uploads: empty CDN URL from resolver (file may require purchase)")
		return fmt.Errorf("empty CDN URL from resolver (file may require purchase)")
	}

	// CDN URL may contain signed tokens — do not log it.
	return c.streamToFile(result.URL, dest, progress)
}

func (c *Client) streamToFile(srcURL, dest string, progress func(int64, int64)) error {
	resp, err := c.http.Get(srcURL)
	if err != nil {
		return fmt.Errorf("fetch file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error("stream: HTTP %d fetching file", resp.StatusCode)
		return fmt.Errorf("file download status %d", resp.StatusCode)
	}

	// Log the destination and size but not the CDN source URL (may contain tokens).
	if resp.ContentLength >= 0 {
		logger.Info("stream: → %s (%d bytes)", dest, resp.ContentLength)
	} else {
		logger.Info("stream: → %s (unknown size)", dest)
	}

	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer f.Close()

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				logger.Error("stream: write error after %d bytes: %v", downloaded, werr)
				return fmt.Errorf("write: %w", werr)
			}
			downloaded += int64(n)
			if progress != nil {
				progress(downloaded, total)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Error("stream: read error after %d bytes: %v", downloaded, err)
			return fmt.Errorf("read stream: %w", err)
		}
	}
	logger.Info("stream: done, wrote %d bytes", downloaded)
	return nil
}
