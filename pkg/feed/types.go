package feed

import (
	"encoding/xml"
)

// RSS represents the root RSS 2.0 element
type RSS struct {
	XMLName xml.Name    `xml:"rss"`
	Version string      `xml:"version,attr"`
	Atom    string      `xml:"xmlns:atom,attr"`
	Channel *RSSChannel `xml:"channel"`
}

// RSSChannel represents an RSS channel
type RSSChannel struct {
	XMLName       xml.Name   `xml:"channel"`
	Title         string     `xml:"title"`
	Link          string     `xml:"link"`
	Description   string     `xml:"description"`
	AtomLink      *AtomLink  `xml:"http://www.w3.org/2005/Atom link"`
	LastBuildDate string     `xml:"lastBuildDate"`
	Items         []*RSSItem `xml:"item"`
}

// AtomLink represents an Atom link element within RSS
type AtomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

// RSSItem represents an item in an RSS feed
type RSSItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	GUID        string   `xml:"guid"`
	Description string   `xml:"description"`
	Author      string   `xml:"author,omitempty"`
	PubDate     string   `xml:"pubDate"`
	Categories  []string `xml:"category"`
}
