package feed

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/umputun/newscope/pkg/domain"
)

// Generator creates RSS feeds from domain items
type Generator struct {
	baseURL string
}

// NewGenerator creates a new feed generator
func NewGenerator(baseURL string) *Generator {
	return &Generator{
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

// GenerateRSS creates an RSS 2.0 feed from classified items
func (g *Generator) GenerateRSS(items []domain.ClassifiedItem, topic string, minScore float64) (string, error) {
	// determine title
	var title string
	if topic != "" {
		title = fmt.Sprintf("Newscope - %s (Score ≥ %.1f)", topic, minScore)
	} else {
		title = fmt.Sprintf("Newscope - All Topics (Score ≥ %.1f)", minScore)
	}

	// build self link
	selfLink := g.baseURL + "/rss"
	if topic != "" {
		selfLink = fmt.Sprintf("%s/rss/%s", g.baseURL, topic)
	}

	// convert items to RSS items
	rssItems := make([]*RSSItem, 0, len(items))
	for _, item := range items {
		rssItem := g.convertToRSSItem(item)
		rssItems = append(rssItems, rssItem)
	}

	// create RSS structure
	feed := &RSS{
		Version: "2.0",
		Atom:    "http://www.w3.org/2005/Atom",
		Channel: &RSSChannel{
			Title:         title,
			Link:          g.baseURL + "/",
			Description:   fmt.Sprintf("AI-curated articles with relevance score ≥ %.1f", minScore),
			AtomLink:      &AtomLink{Href: selfLink, Rel: "self", Type: "application/rss+xml"},
			LastBuildDate: time.Now().Format(time.RFC1123Z),
			Items:         rssItems,
		},
	}

	// marshal to XML
	output, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal RSS: %w", err)
	}

	// add XML declaration
	return xml.Header + string(output), nil
}

// convertToRSSItem converts a domain item with classification to an RSS item
func (g *Generator) convertToRSSItem(item domain.ClassifiedItem) *RSSItem {
	// build description
	desc := fmt.Sprintf("Score: %.1f/10 - %s", item.GetRelevanceScore(), item.GetExplanation())
	topics := item.GetTopics()
	if len(topics) > 0 {
		desc += fmt.Sprintf("\nTopics: %s", strings.Join(topics, ", "))
	}

	// add summary if available, otherwise use original description
	if summary := item.GetSummary(); summary != "" {
		desc += "\n\n" + summary
	} else if item.Description != "" {
		desc += "\n\n" + item.Description
	}

	return &RSSItem{
		Title:       fmt.Sprintf("[%.1f] %s", item.GetRelevanceScore(), item.Title),
		Link:        item.Link,
		GUID:        item.GUID,
		Description: desc,
		Author:      item.Author,
		PubDate:     item.Published.Format(time.RFC1123Z),
		Categories:  topics,
	}
}

// GenerateOPML creates an OPML file with feed subscriptions
func (g *Generator) GenerateOPML(feeds []domain.Feed) (string, error) {
	type outline struct {
		XMLName xml.Name `xml:"outline"`
		Text    string   `xml:"text,attr"`
		Title   string   `xml:"title,attr"`
		Type    string   `xml:"type,attr"`
		XMLUrl  string   `xml:"xmlUrl,attr"`
		HTMLUrl string   `xml:"htmlUrl,attr,omitempty"`
	}

	type body struct {
		XMLName  xml.Name  `xml:"body"`
		Outlines []outline `xml:"outline"`
	}

	type head struct {
		XMLName     xml.Name `xml:"head"`
		Title       string   `xml:"title"`
		DateCreated string   `xml:"dateCreated"`
	}

	type opml struct {
		XMLName xml.Name `xml:"opml"`
		Version string   `xml:"version,attr"`
		Head    head     `xml:"head"`
		Body    body     `xml:"body"`
	}

	// convert feeds to OPML outlines
	outlines := make([]outline, 0, len(feeds))
	for _, feed := range feeds {
		if !feed.Enabled {
			continue
		}
		outlines = append(outlines, outline{
			Text:    feed.Title,
			Title:   feed.Title,
			Type:    "rss",
			XMLUrl:  feed.URL,
			HTMLUrl: feed.URL, // could be improved if we track the website URL separately
		})
	}

	// create OPML structure
	doc := opml{
		Version: "2.0",
		Head: head{
			Title:       "Newscope Feed Subscriptions",
			DateCreated: time.Now().Format(time.RFC1123Z),
		},
		Body: body{
			Outlines: outlines,
		},
	}

	// marshal to XML
	output, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal OPML: %w", err)
	}

	return xml.Header + string(output), nil
}
