package server

import (
	"log"
	"net/http"
	"strconv"

	"github.com/umputun/newscope/pkg/feed"
)

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

	// get base URL from config
	cfg := s.config.GetFullConfig()
	baseURL := cfg.Server.BaseURL

	// create feed generator
	generator := feed.NewGenerator(baseURL)

	// generate RSS feed
	rss, err := generator.GenerateRSS(items, topic, minScore)
	if err != nil {
		log.Printf("[ERROR] failed to generate RSS feed: %v", err)
		http.Error(w, "Failed to generate RSS feed", http.StatusInternalServerError)
		return
	}

	// set content type and write RSS
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	if _, err := w.Write([]byte(rss)); err != nil {
		log.Printf("[ERROR] failed to write RSS response: %v", err)
	}
}
