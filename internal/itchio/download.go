package itchio

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

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
	log.Printf("FetchUploads: signed download URL: %s", dlResult.URL)

	// Step 3: extract the download key from the signed URL path
	// Format: https://author.itch.io/game/download/KEY
	key := extractDownloadKey(dlResult.URL)
	if key == "" {
		return nil, fmt.Errorf("could not extract download key from: %s", dlResult.URL)
	}
	log.Printf("FetchUploads: extracted key: %q", key)

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
		log.Printf("FetchUploads: upload %q id=%s", u.Filename, u.UploadID)
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
	// EscapedPath preserves %-encoding in the raw path, preventing %2F inside
	// the key from being treated as a path separator during the Split below.
	rawPath := parsed.EscapedPath()
	parts := strings.Split(strings.Trim(rawPath, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	// URL-decode just the final segment to recover the actual key string.
	key, err := url.PathUnescape(parts[len(parts)-1])
	if err != nil {
		return parts[len(parts)-1]
	}
	return key
}

// extractKeyID parses the itch.io download key JWT and returns the numeric
// download key ID embedded in its payload.
//
// JWT format: base64({"id":NNNN,"expires":...}).base64(signature)
// itch.io's file resolver endpoint requires this numeric ID as
// "download_key_id", not the full JWT string.
func extractKeyID(jwtKey string) string {
	dotIdx := strings.Index(jwtKey, ".")
	if dotIdx < 0 {
		return ""
	}
	payload := jwtKey[:dotIdx]
	// The payload may or may not have base64 padding.
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
//	gameURL/file/UPLOAD_ID?key=KEY
//
// A POST with key in the form body returns {"url":"CDN_URL"}, which is then
// streamed directly to dest.
func (c *Client) DownloadFree(_ string, upload Upload, dest string, progress func(int64, int64)) error {
	// Parse the resolver URL to extract base path and key
	parsed, err := url.Parse(upload.URL)
	if err != nil {
		return fmt.Errorf("parse resolver URL: %w", err)
	}
	key := parsed.Query().Get("key")
	csrf := parsed.Query().Get("csrf")

	// itch.io's file resolver requires the numeric download key ID (extracted
	// from the JWT payload) as "download_key_id", not the raw JWT string.
	keyID := extractKeyID(key)
	baseURL := parsed.Scheme + "://" + parsed.Host + parsed.Path
	log.Printf("DownloadFree: POST %s (download_key_id=%s csrf=%q)", baseURL, keyID, csrf)

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
		return fmt.Errorf("resolve CDN URL: HTTP %d: %.200s", resp.StatusCode, rawBody)
	}

	var result struct {
		URL    string   `json:"url"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return fmt.Errorf("parse CDN URL response: %w (body: %.200s)", err, rawBody)
	}
	if len(result.Errors) > 0 {
		return fmt.Errorf("resolver error: %s", strings.Join(result.Errors, "; "))
	}
	if result.URL == "" {
		return fmt.Errorf("empty CDN URL from resolver (file may require purchase)")
	}

	log.Printf("DownloadFree: streaming %s → %s", result.URL, dest)
	return c.streamToFile(result.URL, dest, progress)
}

func (c *Client) streamToFile(srcURL, dest string, progress func(int64, int64)) error {
	resp, err := c.http.Get(srcURL)
	if err != nil {
		return fmt.Errorf("fetch file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("file download status %d", resp.StatusCode)
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
			return fmt.Errorf("read stream: %w", err)
		}
	}
	return nil
}
