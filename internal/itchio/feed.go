package itchio

type Game struct {
	Title    string
	Author   string
	URL      string
	CoverURL string
	Price    float64
	IsFree   bool
}

func (c *Client) FetchGames(page int, query string) ([]Game, error)  { return nil, nil }
func (c *Client) FetchGamesFromURL(url string) ([]Game, error)       { return nil, nil }
