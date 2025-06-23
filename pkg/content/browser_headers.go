package content

import (
	"math/rand"
	"net/http"
)

// acceptLanguages contains common browser Accept-Language values
var acceptLanguages = []string{
	"en-US,en;q=0.9",
	"en-GB,en;q=0.9",
	"en-US,en;q=0.9,es;q=0.8",
	"en-US,en;q=0.9,fr;q=0.8",
	"en-US,en;q=0.9,de;q=0.8",
	"en-US,en;q=0.9,ja;q=0.8",
	"en-US,en;q=0.9,zh-CN;q=0.8,zh;q=0.7",
	"en-US,en;q=0.9,ru;q=0.8",
	"fr-FR,fr;q=0.9,en;q=0.8",
	"de-DE,de;q=0.9,en;q=0.8",
	"es-ES,es;q=0.9,en;q=0.8",
}

// secFetchModes for different request contexts
var secFetchModes = []string{
	"navigate",
	"no-cors",
	"cors",
}

// addBrowserHeaders adds common browser headers to the request with some randomization
func addBrowserHeaders(req *http.Request) {
	// essential headers that should always be present
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	// randomized language
	req.Header.Set("Accept-Language", acceptLanguages[rand.Intn(len(acceptLanguages))]) //nolint:gosec // non-cryptographic randomness is fine for header variation

	// dnt - 30% chance of being set
	if rand.Float32() < 0.3 { //nolint:gosec // non-cryptographic randomness is fine
		req.Header.Set("DNT", "1")
	}

	// modern browsers send Sec-Fetch-* headers
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", secFetchModes[rand.Intn(len(secFetchModes))]) //nolint:gosec // non-cryptographic randomness is fine
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")

	// connection header
	if rand.Float32() < 0.8 { //nolint:gosec // non-cryptographic randomness is fine, 80% keep-alive
		req.Header.Set("Connection", "keep-alive")
	}
}
