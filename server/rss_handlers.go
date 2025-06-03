package server

import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/umputun/newscope/pkg/domain"
)

// rss structs for XML encoding
type rssChannel struct {
	XMLName       xml.Name  `xml:"channel"`
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	AtomLink      atomLink  `xml:"http://www.w3.org/2005/Atom link"`
	LastBuildDate string    `xml:"lastBuildDate"`
	Items         []rssItem `xml:"item"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type rssItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	GUID        string   `xml:"guid"`
	Description string   `xml:"description"`
	Author      string   `xml:"author,omitempty"`
	PubDate     string   `xml:"pubDate"`
	Categories  []string `xml:"category"`
}

type rss struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Atom    string     `xml:"xmlns:atom,attr"`
	Channel rssChannel `xml:"channel"`
}

// rssHandler serves RSS feed for articles
// Supports both /rss/{topic} and /rss?topic=... patterns
func (s *Server) rssHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// get topic from path or query params
	topic := r.PathValue("topic")
	if topic == "" {
		topic = r.URL.Query().Get("topic")
	}

	// get min score from query params, default to 5.0
	minScore := defaultMinScore
	if scoreStr := r.URL.Query().Get("min_score"); scoreStr != "" {
		if score, err := strconv.ParseFloat(scoreStr, 64); err == nil {
			minScore = score
		}
	}

	// get classified items
	items, err := s.db.GetClassifiedItems(ctx, minScore, topic, defaultRSSLimit)
	if err != nil {
		log.Printf("[ERROR] failed to get items for RSS: %v", err)
		http.Error(w, "Failed to generate RSS feed", http.StatusInternalServerError)
		return
	}

	// create RSS feed
	rss := s.buildRSSFeed(topic, minScore, items)

	// set content type and write RSS
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	if _, err := w.Write([]byte(rss)); err != nil {
		log.Printf("[ERROR] failed to write RSS response: %v", err)
	}
}

// buildRSSFeed creates an RSS 2.0 feed from classified items
func (s *Server) buildRSSFeed(topic string, minScore float64, items []domain.ItemWithClassification) string {
	// determine title
	var title string
	if topic != "" {
		title = fmt.Sprintf("Newscope - %s (Score ≥ %.1f)", topic, minScore)
	} else {
		title = fmt.Sprintf("Newscope - All Topics (Score ≥ %.1f)", minScore)
	}

	// build self link
	selfLink := defaultBaseURL + "/rss"
	if topic != "" {
		selfLink = fmt.Sprintf("%s/rss/%s", defaultBaseURL, topic)
	}

	// convert items to RSS items
	rssItems := make([]rssItem, 0, len(items))
	for _, item := range items {
		// build description
		desc := fmt.Sprintf("Score: %.1f/10 - %s", item.RelevanceScore, item.Explanation)
		if len(item.Topics) > 0 {
			desc += fmt.Sprintf("\nTopics: %s", strings.Join(item.Topics, ", "))
		}
		if item.Description != "" {
			desc += "\n\n" + item.Description
		}

		rssItems = append(rssItems, rssItem{
			Title:       fmt.Sprintf("[%.1f] %s", item.RelevanceScore, item.Title),
			Link:        item.Link,
			GUID:        item.GUID,
			Description: desc,
			Author:      item.Author,
			PubDate:     item.Published.Format(time.RFC1123Z),
			Categories:  item.Topics,
		})
	}

	// create RSS structure
	feed := rss{
		Version: "2.0",
		Atom:    "http://www.w3.org/2005/Atom",
		Channel: rssChannel{
			Title:         title,
			Link:          defaultBaseURL + "/",
			Description:   fmt.Sprintf("AI-curated articles with relevance score ≥ %.1f", minScore),
			AtomLink:      atomLink{Href: selfLink, Rel: "self", Type: "application/rss+xml"},
			LastBuildDate: time.Now().Format(time.RFC1123Z),
			Items:         rssItems,
		},
	}

	// marshal to XML
	output, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		log.Printf("[ERROR] failed to marshal RSS: %v", err)
		return ""
	}

	// add XML declaration
	return xml.Header + string(output)
}
