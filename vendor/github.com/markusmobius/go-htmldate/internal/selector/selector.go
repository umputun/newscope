package selector

import (
	"github.com/go-shiori/dom"
	"golang.org/x/net/html"
)

type Rule func(*html.Node) bool

// Query find the first element that matched with the rule.
func Query(root *html.Node, selector Rule) *html.Node {
	for _, e := range dom.GetElementsByTagName(root, "*") {
		if selector(e) {
			return e
		}
	}
	return nil
}

// QueryAll find all elements that matched with the rule.
func QueryAll(root *html.Node, selector Rule) []*html.Node {
	var matches []*html.Node
	for _, e := range dom.GetElementsByTagName(root, "*") {
		if selector(e) {
			matches = append(matches, e)
		}
	}
	return matches
}

// QueryAllTextNodes find all text nodes that exist inside elements
// that matched with the rule.
func QueryAllTextNodes(root *html.Node, selector Rule) []*html.Node {
	var matches []*html.Node
	for _, e := range dom.GetElementsByTagName(root, "*") {
		if !selector(e) {
			continue
		}

		for child := e.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.TextNode && child.Data != "" {
				matches = append(matches, child)
			}
		}
	}
	return matches
}
