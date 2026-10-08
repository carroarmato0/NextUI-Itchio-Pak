package itchio

import "time"

// SetStreamIdleTimeoutForTest shortens the download idle timeout.
func SetStreamIdleTimeoutForTest(d time.Duration) func() {
	old := streamIdleTimeout
	streamIdleTimeout = d
	return func() { streamIdleTimeout = old }
}
