package itchio

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"golang.org/x/net/html"
)

// SetAuthToken records the signed-in user's token for background work (the
// update checker), which cannot read the settings the UI goroutine owns.
// "" means signed out.
func (c *Client) SetAuthToken(token string) { c.authToken.Store(&token) }

// AuthToken is the token set by SetAuthToken, or "" when signed out.
func (c *Client) AuthToken() string {
	if t := c.authToken.Load(); t != nil {
		return *t
	}
	return ""
}

// authTokenField is embedded in Client.
type authTokenField struct{ authToken atomic.Pointer[string] }

// FetchPageUploadNames reads the file names a game's public page lists. It is
// a plain GET: unlike the web download flow it triggers no download_url
// request, so it is what update checks use when the user is signed out.
// Files the download flow would skip (Pocket builds, images, documents) are
// left out the same way. A missing game returns ErrGameRemoved.
func (c *Client) FetchPageUploadNames(gameURL string) ([]string, error) {
	resp, err := c.http.Get(gameURL)
	if err != nil {
		return nil, fmt.Errorf("fetch game page: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return nil, fmt.Errorf("fetch game page: %w", ErrGameRemoved)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch game page: HTTP %d", resp.StatusCode)
	}
	doc, err := html.Parse(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("parse game page: %w", err)
	}
	var names []string
	seen := map[string]bool{}
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, inUpload bool) {
		if n.Type == html.ElementNode {
			if nodeHasClass(n, "upload") {
				inUpload = true
			}
			if inUpload && n.Data == "strong" && nodeHasClass(n, "name") {
				name := ""
				for _, a := range n.Attr {
					if a.Key == "title" {
						name = strings.TrimSpace(a.Val)
					}
				}
				if name == "" && n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
					name = strings.TrimSpace(n.FirstChild.Data)
				}
				if name != "" && !seen[name] && !isSkippableExt(strings.ToLower(filepath.Ext(name))) {
					seen[name] = true
					names = append(names, name)
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch, inUpload)
		}
	}
	walk(doc, false)
	logger.Debug("update: public page of %s lists %d file(s)", gameURL, len(names))
	return names, nil
}

// OwnedKeysForGames returns a download key ID for each of gameIDs the user
// owns, asking owned-keys for up to fifty games per request. Games not owned
// are absent from the result.
func (c *Client) OwnedKeysForGames(token string, gameIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(gameIDs))
	for start := 0; start < len(gameIDs); start += maxOwnedKeysGameIDs {
		end := min(start+maxOwnedKeysGameIDs, len(gameIDs))
		keys, err := c.scanOwnedKeys(token, gameIDs[start:end])
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			id := strconv.FormatInt(k.GameID, 10)
			if _, ok := out[id]; !ok { // any key grants access; keep the first
				out[id] = strconv.FormatInt(k.ID, 10)
			}
		}
	}
	logger.Debug("update: %d of %d paid game(s) owned", len(out), len(gameIDs))
	return out, nil
}
