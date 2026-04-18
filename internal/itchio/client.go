package itchio

import (
	"net/http"
	"net/http/cookiejar"
	"time"
)

type Client struct {
	http *http.Client
	base string
}

func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		http: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
		},
		base: "https://itch.io",
	}
}

func NewClientWithBase(base string) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		http: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
		},
		base: base,
	}
}
