package itchio

type GameDetail struct {
	Game
	Description    string
	ScreenshotURLs []string
	Uploads        []Upload
	GameID         string
	CSRFToken      string
}

type Upload struct {
	Filename string
	URL      string
}

func (c *Client) FetchGameDetail(gameURL string) (*GameDetail, error) { return nil, nil }
func (c *Client) ParseDownloadPage(pageURL string) ([]Upload, error)  { return nil, nil }
