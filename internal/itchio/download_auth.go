package itchio

func (c *Client) CheckOwnership(apiKey, gameID string) (bool, error) { return false, nil }
func (c *Client) DownloadAuth(apiKey string, upload Upload, dest string, progress func(int64, int64)) error {
	return nil
}
