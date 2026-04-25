package itchio

import "errors"

// ErrCloudflareBlocked is returned when itch.io responds with HTTP 403,
// indicating Cloudflare bot-protection rejected the request.
var ErrCloudflareBlocked = errors.New("Cloudflare blocked the request (HTTP 403)")
