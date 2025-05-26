// Copyright (C) 2022 Markus Mobius
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code in this file is ported from <https://github.com/adbar/htmldate>
// which available under Apache 2.0 license.

package htmldate

import (
	"bytes"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-shiori/dom"
	"golang.org/x/net/html"
)

// cleanDocument cleans the document by discarding unwanted elements.
func cleanDocument(doc *html.Node) *html.Node {
	// Clone doc
	clone := dom.Clone(doc, true)

	// Remove comments
	// removeHtmlCommentNode(clone)

	// Remove useless nodes
	tagNames := []string{
		// Embed elements
		"object", "embed", "applet",
		// Frame elements
		"frame", "frameset", "noframes", "iframe",
		// Others
		"label", "map", "math",
		"audio", "canvas", "datalist",
		"picture", "rdf", "svg", "track", "video",
		// TODO: to be considered
		// "figure", "input", "layer", "param", "source"
	}

	for _, node := range dom.GetAllNodesWithTag(clone, tagNames...) {
		if node.Parent != nil {
			node.Parent.RemoveChild(node)
		}
	}

	return clone
}

// removeHtmlCommentNode removes all `html.CommentNode` in document.
func removeHtmlCommentNode(doc *html.Node) {
	// Find all comment nodes
	var finder func(*html.Node)
	var commentNodes []*html.Node

	finder = func(node *html.Node) {
		if node.Type == html.CommentNode {
			commentNodes = append(commentNodes, node)
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			finder(child)
		}
	}

	for child := doc.FirstChild; child != nil; child = child.NextSibling {
		finder(child)
	}

	// Remove it
	dom.RemoveNodes(commentNodes, nil)
}

// isDigit check if string only consisted of digit number.
func isDigit(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}

	return true
}

// getDigitCount returns count of digit number in the specified string.
func getDigitCount(s string) int {
	var nDigit int
	for _, r := range s {
		if unicode.IsDigit(r) {
			nDigit++
		}
	}
	return nDigit
}

// etreeText returns texts before first subelement. If there was no text,
// this function will returns an empty string.
func etreeText(element *html.Node) string {
	if element == nil {
		return ""
	}

	buffer := bytes.NewBuffer(nil)
	for child := element.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			break
		} else if child.Type == html.TextNode {
			buffer.WriteString(child.Data)
		}
	}

	return buffer.String()
}

// inMap check if keys exist in map.
func inMap(key string, mapString map[string]struct{}) bool {
	_, exist := mapString[key]
	return exist
}

// strLimit cut a string until the specified limit.
func strLimit(s string, limit int) string {
	if utf8.RuneCountInString(s) > limit {
		s = string([]rune(s)[:limit])
	}

	return s
}

// normalizeSpaces converts all whitespaces to normal spaces, remove multiple adjacent
// whitespaces and trim the string.
func normalizeSpaces(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

func rxFindNamedStringSubmatch(rx *regexp.Regexp, s string) (map[string]string, string) {
	names := rx.SubexpNames()
	result := make(map[string]string)
	matches := rx.FindStringSubmatch(s)

	var lastMatchedName string
	for i, match := range matches {
		if i > 0 && match != "" {
			result[names[i]] = match
			lastMatchedName = names[i]
		}
	}

	return result, lastMatchedName
}

// isLeapYear check if year is leap year.
func isLeapYear(year int) bool {
	// If year is not divisible by 4, then it is not a leap year
	if year%4 != 0 {
		return false
	}

	// If year is not divisible by 100, then it is a leap year
	if year%100 != 0 {
		return true
	}

	// If year is not divisible by 400, then it is not a leap year
	if year%400 != 0 {
		return false
	}

	// If all passed, it's leap year
	return true
}
