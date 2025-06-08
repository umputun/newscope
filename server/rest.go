package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/umputun/newscope/pkg/domain"
)

// statusHandler returns server status
func (s *Server) statusHandler(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":  "ok",
		"version": s.version,
		"time":    time.Now().UTC(),
	}
	renderJSON(w, r, http.StatusOK, status)
}

// feedbackHandler handles user feedback (like/dislike)
func (s *Server) feedbackHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := r.PathValue("id")
	action := r.PathValue("action")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		renderError(w, r, fmt.Errorf("invalid item ID"), http.StatusBadRequest)
		return
	}

	// validate action
	if action != "like" && action != "dislike" {
		renderError(w, r, fmt.Errorf("invalid action"), http.StatusBadRequest)
		return
	}

	// update feedback
	if err := s.db.UpdateItemFeedback(ctx, id, action); err != nil {
		log.Printf("[ERROR] failed to update feedback: %v", err)
		renderError(w, r, err, http.StatusInternalServerError)
		return
	}

	// trigger preference summary update in background
	s.scheduler.TriggerPreferenceUpdate()

	// get the updated article
	article, err := s.db.GetClassifiedItem(ctx, id)
	if err != nil {
		log.Printf("[ERROR] failed to get article after feedback: %v", err)
		http.Error(w, "Failed to reload article", http.StatusInternalServerError)
		return
	}

	// check if this is an HTMX request
	if r.Header.Get("HX-Request") == "true" {
		// get current filter parameters from form data (included by hx-include)
		minScore := 0.0
		if scoreStr := r.FormValue("score"); scoreStr != "" {
			if score, err := strconv.ParseFloat(scoreStr, 64); err == nil {
				minScore = score
			}
		}

		// if article no longer meets score threshold after feedback, remove it
		if article.RelevanceScore < minScore {
			// return empty response to remove the article from the list
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	// return the updated article card HTML
	s.renderArticleCard(w, article)
}

// extractHandler triggers content extraction for an item
func (s *Server) extractHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		renderError(w, r, fmt.Errorf("invalid item ID"), http.StatusBadRequest)
		return
	}

	// trigger extraction
	if err := s.scheduler.ExtractContentNow(ctx, id); err != nil {
		log.Printf("[ERROR] failed to extract content: %v", err)
		renderError(w, r, err, http.StatusInternalServerError)
		return
	}

	// get the updated article
	article, err := s.db.GetClassifiedItem(ctx, id)
	if err != nil {
		log.Printf("[ERROR] failed to get article after extraction: %v", err)
		http.Error(w, "Failed to reload article", http.StatusInternalServerError)
		return
	}

	// for HTMX, return the updated article card HTML
	s.renderArticleCard(w, article)
}

// createFeedHandler handles feed creation
func (s *Server) createFeedHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// parse form data
	err := r.ParseForm()
	if err != nil {
		renderError(w, r, fmt.Errorf("invalid form data"), http.StatusBadRequest)
		return
	}

	url := r.FormValue("url")
	if url == "" {
		renderError(w, r, fmt.Errorf("feed URL is required"), http.StatusBadRequest)
		return
	}

	// parse fetch interval
	fetchInterval := 30 * time.Minute // default 30 minutes
	if intervalStr := r.FormValue("fetch_interval"); intervalStr != "" {
		if minutes, err := strconv.Atoi(intervalStr); err == nil {
			fetchInterval = time.Duration(minutes) * time.Minute
		}
	}

	feed := &domain.Feed{
		URL:           url,
		Title:         r.FormValue("title"),
		FetchInterval: fetchInterval,
		Enabled:       true,
	}

	// create feed in database
	if err := s.db.CreateFeed(ctx, feed); err != nil {
		log.Printf("[ERROR] failed to create feed: %v", err)
		renderError(w, r, err, http.StatusInternalServerError)
		return
	}

	// trigger immediate fetch
	go func() {
		if err := s.scheduler.UpdateFeedNow(context.Background(), feed.ID); err != nil {
			log.Printf("[ERROR] failed to fetch new feed: %v", err)
		}
	}()

	// return the feed card HTML for HTMX
	s.renderFeedCard(w, feed)
}

// updateFeedHandler updates feed title and interval
func (s *Server) updateFeedHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		renderError(w, r, fmt.Errorf("invalid feed ID"), http.StatusBadRequest)
		return
	}

	// parse form data
	err = r.ParseForm()
	if err != nil {
		renderError(w, r, fmt.Errorf("invalid form data"), http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")

	// parse fetch interval
	fetchInterval := 30 * time.Minute // default 30 minutes
	if intervalStr := r.FormValue("fetch_interval"); intervalStr != "" {
		if minutes, err := strconv.Atoi(intervalStr); err == nil {
			fetchInterval = time.Duration(minutes) * time.Minute
		}
	}

	// update feed
	if err := s.db.UpdateFeed(ctx, id, title, fetchInterval); err != nil {
		log.Printf("[ERROR] failed to update feed: %v", err)
		renderError(w, r, err, http.StatusInternalServerError)
		return
	}

	// get updated feed
	feeds, err := s.db.GetAllFeeds(ctx)
	if err != nil {
		http.Error(w, "Failed to reload feed", http.StatusInternalServerError)
		return
	}

	// find the updated feed
	for _, feed := range feeds {
		if feed.ID == id {
			s.renderFeedCard(w, &feed)
			return
		}
	}

	http.Error(w, "Feed not found", http.StatusNotFound)
}

// enableFeedHandler enables a feed
func (s *Server) enableFeedHandler(w http.ResponseWriter, r *http.Request) {
	s.updateFeedStatus(w, r, true)
}

// disableFeedHandler disables a feed
func (s *Server) disableFeedHandler(w http.ResponseWriter, r *http.Request) {
	s.updateFeedStatus(w, r, false)
}

// updateFeedStatus updates feed enabled status
func (s *Server) updateFeedStatus(w http.ResponseWriter, r *http.Request, enabled bool) {
	ctx := r.Context()

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		renderError(w, r, fmt.Errorf("invalid feed ID"), http.StatusBadRequest)
		return
	}

	// update status
	if err := s.db.UpdateFeedStatus(ctx, id, enabled); err != nil {
		log.Printf("[ERROR] failed to update feed status: %v", err)
		renderError(w, r, err, http.StatusInternalServerError)
		return
	}

	// get updated feed
	feeds, err := s.db.GetAllFeeds(ctx)
	if err != nil {
		http.Error(w, "Failed to reload feed", http.StatusInternalServerError)
		return
	}

	// find the updated feed
	for _, feed := range feeds {
		if feed.ID == id {
			s.renderFeedCard(w, &feed)
			return
		}
	}

	http.Error(w, "Feed not found", http.StatusNotFound)
}

// fetchFeedHandler triggers immediate feed fetch
func (s *Server) fetchFeedHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		renderError(w, r, fmt.Errorf("invalid feed ID"), http.StatusBadRequest)
		return
	}

	// trigger fetch with background context to avoid cancellation when HTTP request completes
	if err := s.scheduler.UpdateFeedNow(context.Background(), id); err != nil {
		log.Printf("[ERROR] failed to fetch feed: %v", err)
		renderError(w, r, err, http.StatusInternalServerError)
		return
	}

	// get updated feed to show new fetch time
	feeds, err := s.db.GetAllFeeds(ctx)
	if err != nil {
		log.Printf("[ERROR] failed to get feed after fetch: %v", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	// find the updated feed
	for _, feed := range feeds {
		if feed.ID == id {
			s.renderFeedCard(w, &feed)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

// deleteFeedHandler deletes a feed
func (s *Server) deleteFeedHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		renderError(w, r, fmt.Errorf("invalid feed ID"), http.StatusBadRequest)
		return
	}

	// delete feed
	if err := s.db.DeleteFeed(ctx, id); err != nil {
		log.Printf("[ERROR] failed to delete feed: %v", err)
		renderError(w, r, err, http.StatusInternalServerError)
		return
	}

	// return empty response for HTMX to remove the element
	w.WriteHeader(http.StatusOK)
}

// rssBuilderHandler handles HTMX requests for RSS URL building
func (s *Server) rssBuilderHandler(w http.ResponseWriter, r *http.Request) {
	topic := r.URL.Query().Get("topic")
	score := r.URL.Query().Get("score")
	if score == "" {
		score = "5.0"
	}

	// build RSS URL
	url := "/rss"
	if topic != "" {
		url = fmt.Sprintf("/rss/%s", topic)
	}

	// add score parameter if not default
	if score != "5.0" {
		if strings.Contains(url, "?") {
			url += fmt.Sprintf("&min_score=%s", score)
		} else {
			url += fmt.Sprintf("?min_score=%s", score)
		}
	}

	// return just the URL text for HTMX to update
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, url)
}

// renderJSON sends JSON response
func renderJSON(w http.ResponseWriter, _ *http.Request, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			log.Printf("[ERROR] can't encode response to JSON: %v", err)
		}
	}
}

// renderError sends error response as JSON
func renderError(w http.ResponseWriter, r *http.Request, err error, code int) {
	errMsg := "unknown error"
	if err != nil {
		errMsg = err.Error()
	}
	renderJSON(w, r, code, map[string]string{"error": errMsg})
}
