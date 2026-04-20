package itchio

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// FetchAuthUploads lists .gb/.gbc uploads for a paid game the user owns,
// using the itch.io REST API. Returns the uploads and the numeric download
// key ID required to resolve CDN URLs.
func (c *Client) FetchAuthUploads(apiKey, gameID string) ([]Upload, string, error) {
	// Step 1: get download key ID — proves ownership and is required for CDN resolution.
	keysURL := fmt.Sprintf("%s/api/1/%s/game/%s/download_keys", c.base, apiKey, gameID)
	resp, err := c.http.Get(keysURL)
	if err != nil {
		return nil, "", fmt.Errorf("fetch download keys: %w", err)
	}
	defer resp.Body.Close()
	var keysResult struct {
		DownloadKeys []struct {
			ID int64 `json:"id"`
		} `json:"download_keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&keysResult); err != nil {
		return nil, "", fmt.Errorf("decode download keys: %w", err)
	}
	if len(keysResult.DownloadKeys) == 0 {
		return nil, "", fmt.Errorf("game not owned or API key invalid (no download key found)")
	}
	downloadKeyID := fmt.Sprintf("%d", keysResult.DownloadKeys[0].ID)
	log.Printf("FetchAuthUploads: download_key_id=%s", downloadKeyID)

	// Step 2: list uploads via the API.
	uploadsURL := fmt.Sprintf("%s/api/1/%s/game/%s/uploads", c.base, apiKey, gameID)
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

// DownloadAuthUpload resolves the CDN URL for an owned upload via the itch.io
// API and streams it to dest.
func (c *Client) DownloadAuthUpload(apiKey, uploadID, downloadKeyID, dest string, progress func(int64, int64)) error {
	dlURL := fmt.Sprintf("%s/api/1/%s/upload/%s/download?download_key_id=%s",
		c.base, apiKey, uploadID, downloadKeyID)
	log.Printf("DownloadAuthUpload: resolving CDN URL from %s", dlURL)

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

	log.Printf("DownloadAuthUpload: streaming %s → %s", result.URL, dest)
	return c.streamToFile(result.URL, dest, progress)
}

func (c *Client) CheckOwnership(apiKey, gameID string) (bool, error) {
	url := fmt.Sprintf("%s/api/1/%s/game/%s/download_keys", c.base, apiKey, gameID)
	resp, err := c.http.Get(url)
	if err != nil {
		return false, fmt.Errorf("check ownership: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	var result struct {
		DownloadKeys []struct {
			ID int `json:"id"`
		} `json:"download_keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("decode ownership response: %w", err)
	}
	return len(result.DownloadKeys) > 0, nil
}

func (c *Client) DownloadAuth(apiKey string, upload Upload, dest string, progress func(int64, int64)) error {
	req, err := http.NewRequest("GET", upload.URL, nil)
	if err != nil {
		return fmt.Errorf("build auth request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("auth download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("not authorized (status %d) — check API key and game ownership", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth download status %d", resp.StatusCode)
	}

	// Stream directly from the authenticated response body — do NOT call
	// streamToFile(upload.URL, ...) which would issue a second unauthenticated GET.
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
