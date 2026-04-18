//go:build headless

package main

// runSDL is a no-op stub used when building with -tags headless (CI / unit tests).
func runSDL() {}
