package itchio

import "net/http"

type Client struct {
	http *http.Client
	base string
}

func NewClient() *Client                    { return nil }
func NewClientWithBase(base string) *Client { return nil }
