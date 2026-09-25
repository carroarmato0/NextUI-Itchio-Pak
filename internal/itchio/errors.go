package itchio

import "errors"

// ErrCloudflareBlocked is returned when itch.io responds with HTTP 403,
// indicating Cloudflare bot-protection rejected the request.
var ErrCloudflareBlocked = errors.New("Cloudflare blocked the request (HTTP 403)")

// ErrGameRemoved is returned when the game page responds with HTTP 404 or 410.
var ErrGameRemoved = errors.New("game removed (HTTP 404/410)")

// ErrFeedPageNotFound is returned when a feed page responds with HTTP 404 or
// 410. Past page 1 it means the feed has ended, not that anything failed.
var ErrFeedPageNotFound = errors.New("feed page not found (HTTP 404/410)")

// ErrTokenRejected is returned when itch.io refuses the sign-in token: it was
// revoked on itch.io, or is otherwise no longer valid. The user has to sign in
// again. Network failures are never reported as this.
var ErrTokenRejected = errors.New("itch.io no longer accepts this sign-in")
