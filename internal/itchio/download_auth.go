package itchio

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

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
