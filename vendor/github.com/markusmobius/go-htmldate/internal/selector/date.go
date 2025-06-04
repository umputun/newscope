package selector

import (
	"strings"

	"github.com/go-shiori/dom"
	"golang.org/x/net/html"
)

// `.//footer | .//small | //*`, then date selector.
func SlowDate(n *html.Node) bool {
	switch dom.TagName(n) {
	case "footer", "small":
		return true
	default:
		return dateRule(n)
	}
}

// ..//footer | .//small | //*[self::div or self::h2 or self::h3 or self::h4 or self::li or self::p or self::span or self::time or self::ul], then date selector.
func FastDate(n *html.Node) bool {
	switch dom.TagName(n) {
	case "footer", "small":
		return true
	case "div", "h2", "h3", "h4", "li", "p", "span", "time", "ul":
		return dateRule(n)
	default:
		return false
	}
}

// [
// contains(translate(@id|@class|@itemprop, "D", "d"), 'date') or
// contains(translate(@id|@class|@itemprop, "D", "d"), 'datum') or
// contains(translate(@id|@class, "M", "m"), 'meta') or
// contains(@id|@class, 'time') or
// contains(@id|@class, 'publish') or
// contains(@id|@class, 'footer') or
// contains(@class, 'info') or
// contains(@class, 'post_detail') or
// contains(@class, 'block-content') or
// contains(@class, 'byline') or
// contains(@class, 'subline') or
// contains(@class, 'posted') or
// contains(@class, 'submitted') or
// contains(@class, 'created-post') or
// contains(@class, 'publication') or
// contains(@class, 'author') or
// contains(@class, 'autor') or
// contains(@class, 'field-content') or
// contains(@class, 'fa-clock-o') or
// contains(@class, 'fa-calendar') or
// contains(@class, 'fecha') or
// contains(@class, 'parution') or
// contains(@id, 'footer-info-lastmod')
// ]
//
// Further tests needed:
// or contains(@class, 'article')
// or contains(@class, 'footer') or contains(@id, 'footer')
// or contains(@id, 'lastmod') or contains(@class, 'updated')
func dateRule(n *html.Node) bool {
	id := dom.ID(n)
	class := dom.ClassName(n)
	itemProp := dom.GetAttribute(n, "itemprop")

	or := strOr
	contains := strings.Contains
	translate := strings.ReplaceAll

	switch {
	case contains(translate(or(id, class, itemProp), "D", "d"), "date"),
		contains(translate(or(id, class, itemProp), "D", "d"), "datum"),
		contains(translate(or(id, class), "M", "m"), "meta"),
		contains(or(id, class), "time"),
		contains(or(id, class), "publish"),
		contains(or(id, class), "footer"),
		contains(class, "info"),
		contains(class, "post_detail"),
		contains(class, "block-content"),
		contains(class, "byline"),
		contains(class, "subline"),
		contains(class, "posted"),
		contains(class, "submitted"),
		contains(class, "created-post"),
		contains(class, "publication"),
		contains(class, "author"),
		contains(class, "autor"),
		contains(class, "field-content"),
		contains(class, "fa-clock-o"),
		contains(class, "fa-calendar"),
		contains(class, "fecha"),
		contains(class, "parution"),
		contains(id, "footer-info-lastmod"):
		return true
	default:
		return false
	}
}

func strOr(strs ...string) string {
	for i := range strs {
		if strs[i] != "" {
			return strs[i]
		}
	}
	return ""
}
