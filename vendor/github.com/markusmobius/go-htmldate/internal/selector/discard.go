package selector

import (
	"github.com/go-shiori/dom"
	"golang.org/x/net/html"
)

// `.//div[@id="wm-ipp-base" or @id="wm-ipp"]`
func Discard(n *html.Node) bool {
	switch dom.TagName(n) {
	case "div":
	default:
		return false
	}

	switch dom.ID(n) {
	case "wm-ipp-base", "wm-ipp":
		return true
	default:
		return false
	}
}
