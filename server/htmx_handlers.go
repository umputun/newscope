package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/umputun/newscope/pkg/config"
	"github.com/umputun/newscope/pkg/domain"
)

// articlesPageRequest holds data for rendering articles page
type articlesPageRequest struct {
	articles      []domain.ItemWithClassification
	topics        []string
	feeds         []string
	selectedTopic string
	selectedFeed  string
	selectedSort  string
	showLikedOnly bool
	// pagination
	currentPage int
	totalPages  int
	totalCount  int
	pageSize    int
	pageNumbers []int
	hasNext     bool
	hasPrev     bool
	minScore    float64
}

// articlesHandler displays the main articles page
func (s *Server) articlesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// get query parameters
	minScore := 0.0
	if scoreStr := r.URL.Query().Get("score"); scoreStr != "" {
		if score, err := strconv.ParseFloat(scoreStr, 64); err == nil {
			minScore = score
		}
	}
	topic := r.URL.Query().Get("topic")
	feedName := r.URL.Query().Get("feed")
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "published" // default sort
	}
	showLikedOnly := r.URL.Query().Get("liked") == "true" || r.URL.Query().Get("liked") == "on"

	// get page parameter
	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	// get articles with classification
	pageSize := s.GetPageSize()
	req := domain.ArticlesRequest{
		MinScore:      minScore,
		Topic:         topic,
		FeedName:      feedName,
		SortBy:        sortBy,
		Limit:         pageSize,
		Page:          page,
		ShowLikedOnly: showLikedOnly,
	}
	articles, err := s.db.GetClassifiedItemsWithFilters(ctx, req)
	if err != nil {
		log.Printf("[ERROR] failed to get classified items: %v", err)
		http.Error(w, "Failed to load articles", http.StatusInternalServerError)
		return
	}

	// get total count for pagination
	totalCount, err := s.db.GetClassifiedItemsCount(ctx, req)
	if err != nil {
		log.Printf("[ERROR] failed to get classified items count: %v", err)
		http.Error(w, "Failed to load articles count", http.StatusInternalServerError)
		return
	}

	// calculate pagination info
	totalPages := (totalCount + pageSize - 1) / pageSize
	hasNext := page < totalPages
	hasPrev := page > 1

	// get topics filtered by current score
	topics, err := s.db.GetTopicsFiltered(ctx, minScore)
	if err != nil {
		log.Printf("[ERROR] failed to get topics: %v", err)
		topics = []string{} // continue with empty topics
	}

	// get active feed names
	feeds, err := s.db.GetActiveFeedNames(ctx, minScore)
	if err != nil {
		log.Printf("[ERROR] failed to get feed names: %v", err)
		feeds = []string{} // continue with empty feeds
	}

	// check if this is an HTMX request for partial update
	if r.Header.Get("HX-Request") == "true" {
		s.handleHTMXArticlesRequest(w, articlesPageRequest{
			articles:      articles,
			topics:        topics,
			feeds:         feeds,
			selectedTopic: topic,
			selectedFeed:  feedName,
			selectedSort:  sortBy,
			showLikedOnly: showLikedOnly,
			// pagination
			currentPage: page,
			totalPages:  totalPages,
			totalCount:  totalCount,
			pageSize:    pageSize,
			pageNumbers: generatePageNumbers(page, totalPages),
			hasNext:     hasNext,
			hasPrev:     hasPrev,
			minScore:    minScore,
		})
		return
	}

	// prepare template data for full page render
	data := struct {
		ActivePage    string
		Articles      []domain.ItemWithClassification
		ArticleCount  int
		TotalCount    int
		Topics        []string
		Feeds         []string
		MinScore      float64
		SelectedTopic string
		SelectedFeed  string
		SelectedSort  string
		ShowLikedOnly bool
		// pagination
		CurrentPage int
		TotalPages  int
		PageSize    int
		PageNumbers []int
		HasNext     bool
		HasPrev     bool
		IsHTMX      bool
	}{
		ActivePage:    "home",
		Articles:      articles,
		ArticleCount:  len(articles),
		TotalCount:    totalCount,
		Topics:        topics,
		Feeds:         feeds,
		MinScore:      minScore,
		SelectedTopic: topic,
		SelectedFeed:  feedName,
		SelectedSort:  sortBy,
		ShowLikedOnly: showLikedOnly,
		// pagination
		CurrentPage: page,
		TotalPages:  totalPages,
		PageSize:    pageSize,
		PageNumbers: generatePageNumbers(page, totalPages),
		HasNext:     hasNext,
		HasPrev:     hasPrev,
		IsHTMX:      false,
	}

	// render full page with base template and article card component
	if err := s.renderPage(w, "articles.html", data); err != nil {
		log.Printf("[ERROR] failed to render articles page: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// handleHTMXArticlesRequest handles HTMX requests for articles page
func (s *Server) handleHTMXArticlesRequest(w http.ResponseWriter, req articlesPageRequest) {
	// for HTMX requests, return updated count, topic dropdown, feed dropdown, and articles with pagination
	// first update the count using out-of-band swap
	if _, err := fmt.Fprintf(w, `<span id="article-count" class="article-count" hx-swap-oob="true">(%d/%d)</span>`, len(req.articles), req.totalCount); err != nil {
		log.Printf("[ERROR] failed to write article count: %v", err)
	}

	// update topic dropdown using out-of-band swap
	s.writeTopicDropdown(w, req.topics, req.selectedTopic)

	// update feed dropdown using out-of-band swap
	s.writeFeedDropdown(w, req.feeds, req.selectedFeed)

	// update liked button state using out-of-band swap
	s.writeLikedButton(w, req.showLikedOnly)

	// render the complete articles-with-pagination wrapper
	if _, err := w.Write([]byte(`<div id="articles-container" class="view-expanded"><div id="articles-list">`)); err != nil {
		log.Printf("[ERROR] failed to write articles container start: %v", err)
	}

	for i := range req.articles {
		s.renderArticleCard(w, &req.articles[i])
	}

	if len(req.articles) == 0 {
		if _, err := w.Write([]byte(`<p class="no-articles">No articles found. Try lowering the score filter or wait for classification to run.</p>`)); err != nil {
			log.Printf("[ERROR] failed to write no articles message: %v", err)
		}
	}

	if _, err := w.Write([]byte(`</div></div>`)); err != nil {
		log.Printf("[ERROR] failed to write articles container end: %v", err)
	}

	// render pagination controls directly (not out-of-band)
	s.writePaginationControls(w, req)
}

// feedsHandler displays the feeds management page
func (s *Server) feedsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// get all feeds from database
	feeds, err := s.db.GetAllFeeds(ctx)
	if err != nil {
		log.Printf("[ERROR] failed to get feeds: %v", err)
		http.Error(w, "Failed to load feeds", http.StatusInternalServerError)
		return
	}

	// prepare template data
	data := struct {
		ActivePage string
		Feeds      []domain.Feed
	}{
		ActivePage: "feeds",
		Feeds:      feeds,
	}

	// render page with base template
	if err := s.renderPage(w, "feeds.html", data); err != nil {
		log.Printf("[ERROR] failed to render feeds page: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// settingsHandler displays the settings page
func (s *Server) settingsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// get full configuration
	cfg := s.config.GetFullConfig()

	// get topic preferences from database
	var preferredTopics, avoidedTopics []string

	if preferredJSON, err := s.db.GetSetting(ctx, domain.SettingPreferredTopics); err == nil && preferredJSON != "" {
		if err := json.Unmarshal([]byte(preferredJSON), &preferredTopics); err != nil {
			log.Printf("[ERROR] failed to parse preferred topics: %v", err)
		}
	}

	if avoidedJSON, err := s.db.GetSetting(ctx, domain.SettingAvoidedTopics); err == nil && avoidedJSON != "" {
		if err := json.Unmarshal([]byte(avoidedJSON), &avoidedTopics); err != nil {
			log.Printf("[ERROR] failed to parse avoided topics: %v", err)
		}
	}

	// get all available topics for the dropdown
	availableTopics, err := s.db.GetTopics(ctx)
	if err != nil {
		log.Printf("[ERROR] failed to get available topics: %v", err)
		availableTopics = []string{} // continue with empty topics
	}

	// prepare data for display
	data := struct {
		ActivePage      string
		Config          *config.Config
		Version         string
		Debug           bool
		PreferredTopics []string
		AvoidedTopics   []string
		AvailableTopics []string
	}{
		ActivePage:      "settings",
		Config:          cfg,
		Version:         s.version,
		Debug:           s.debug,
		PreferredTopics: preferredTopics,
		AvoidedTopics:   avoidedTopics,
		AvailableTopics: availableTopics,
	}

	// render settings page
	if err := s.renderPage(w, "settings.html", data); err != nil {
		log.Printf("[ERROR] failed to render settings page: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// rssHelpHandler displays the RSS help/documentation page
func (s *Server) rssHelpHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// get top 10 topics sorted by average score for display
	topTopics, err := s.db.GetTopTopicsByScore(ctx, 5.0, 10) // min score 5.0, top 10
	if err != nil {
		log.Printf("[ERROR] failed to get top topics for RSS help: %v", err)
		topTopics = []domain.TopicWithScore{} // continue with empty topics
	}

	// get all topics for the RSS builder dropdown
	allTopics, err := s.db.GetTopics(ctx)
	if err != nil {
		log.Printf("[ERROR] failed to get all topics for RSS help: %v", err)
		allTopics = []string{} // continue with empty topics
	}

	// get base URL from config
	cfg := s.config.GetFullConfig()
	baseURL := cfg.Server.BaseURL

	// prepare template data
	data := struct {
		ActivePage string
		TopTopics  []domain.TopicWithScore
		AllTopics  []string
		BaseURL    string
		Version    string
	}{
		ActivePage: "rss-help",
		TopTopics:  topTopics,
		AllTopics:  allTopics,
		BaseURL:    baseURL,
		Version:    s.version,
	}

	// render RSS help page
	if err := s.renderPage(w, "rss-help.html", data); err != nil {
		log.Printf("[ERROR] failed to render RSS help page: %v", err)
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
	}
}

// articleContentHandler returns extracted content for an article
func (s *Server) articleContentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid article ID", http.StatusBadRequest)
		return
	}

	// get the article with classification
	article, err := s.db.GetClassifiedItem(ctx, id)
	if err != nil {
		log.Printf("[ERROR] failed to get article: %v", err)
		http.Error(w, "Article not found", http.StatusNotFound)
		return
	}

	// render the content template
	if err := s.templates.ExecuteTemplate(w, "article-content.html", article); err != nil {
		log.Printf("[ERROR] failed to render article content: %v", err)
		http.Error(w, "Failed to render content", http.StatusInternalServerError)
		return
	}

	// also send out-of-band update for the button
	fmt.Fprintf(w, `<span id="content-toggle-%d" hx-swap-oob="true">
		<button class="btn-content"
			hx-get="/api/v1/articles/%d/hide"
			hx-target="#content-%d"
			hx-swap="innerHTML">
			Hide Content
		</button>
	</span>`, id, id, id)
}

// hideContentHandler returns the hidden state for article content
func (s *Server) hideContentHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid article ID", http.StatusBadRequest)
		return
	}

	// clear the content div
	if _, err := w.Write([]byte("")); err != nil {
		log.Printf("[ERROR] failed to write response: %v", err)
	}

	// also send out-of-band update for the button
	fmt.Fprintf(w, `<span id="content-toggle-%d" hx-swap-oob="true">
		<button class="btn-content"
			hx-get="/api/v1/articles/%d/content"
			hx-target="#content-%d"
			hx-swap="innerHTML">
			Show Content
		</button>
	</span>`, id, id, id)
}

// renderPage renders a pre-parsed page template
func (s *Server) renderPage(w http.ResponseWriter, templateName string, data interface{}) error {
	// get the pre-parsed template
	tmpl, ok := s.pageTemplates[templateName]
	if !ok {
		return fmt.Errorf("template %s not found", templateName)
	}

	// execute the template
	return tmpl.ExecuteTemplate(w, templateName, data)
}

// renderArticleCard renders a single article card as HTML
func (s *Server) renderArticleCard(w http.ResponseWriter, article *domain.ItemWithClassification) {
	if err := s.templates.ExecuteTemplate(w, "article-card.html", article); err != nil {
		log.Printf("[ERROR] failed to render article card: %v", err)
		http.Error(w, "Failed to render article", http.StatusInternalServerError)
	}
}

// renderFeedCard renders a single feed card
func (s *Server) renderFeedCard(w http.ResponseWriter, feed *domain.Feed) {
	if err := s.templates.ExecuteTemplate(w, "feed-card.html", feed); err != nil {
		log.Printf("[ERROR] failed to render feed card: %v", err)
		http.Error(w, "Failed to render feed", http.StatusInternalServerError)
	}
}

// writeTopicDropdown writes the topic dropdown HTML
func (s *Server) writeTopicDropdown(w http.ResponseWriter, topics []string, selectedTopic string) {
	var topicHTML strings.Builder
	topicHTML.WriteString(`<select id="topic-filter" name="topic" hx-get="/articles" hx-trigger="change" hx-target="#articles-with-pagination" hx-include="#score-filter, #feed-filter" hx-swap-oob="true">`)
	topicHTML.WriteString(`<option value="">All Topics</option>`)

	for _, t := range topics {
		selected := ""
		if t == selectedTopic {
			selected = " selected"
		}
		topicHTML.WriteString(fmt.Sprintf(`<option value=%q%s>%s</option>`, t, selected, t))
	}

	topicHTML.WriteString(`</select>`)

	if _, err := w.Write([]byte(topicHTML.String())); err != nil {
		log.Printf("[ERROR] failed to write topic dropdown: %v", err)
	}
}

// writeFeedDropdown writes the feed dropdown HTML
func (s *Server) writeFeedDropdown(w http.ResponseWriter, feeds []string, selectedFeed string) {
	var feedHTML strings.Builder
	feedHTML.WriteString(`<select id="feed-filter" name="feed" hx-get="/articles" hx-trigger="change" hx-target="#articles-with-pagination" hx-include="#score-filter, #topic-filter" hx-swap-oob="true">`)
	feedHTML.WriteString(`<option value="">All Feeds</option>`)

	for _, f := range feeds {
		selected := ""
		if f == selectedFeed {
			selected = " selected"
		}
		feedHTML.WriteString(fmt.Sprintf(`<option value=%q%s>%s</option>`, f, selected, f))
	}

	feedHTML.WriteString(`</select>`)

	if _, err := w.Write([]byte(feedHTML.String())); err != nil {
		log.Printf("[ERROR] failed to write feed dropdown: %v", err)
	}
}

// writeLikedButton writes the liked button with proper state using out-of-band swap
func (s *Server) writeLikedButton(w http.ResponseWriter, showLikedOnly bool) {
	activeClass := ""
	nextValue := "true"
	if showLikedOnly {
		activeClass = " active"
		nextValue = "false"
	}

	buttonHTML := fmt.Sprintf(`<button id="liked-toggle" class="btn-toggle%s" 
                    title="Show liked articles only"
                    hx-get="/articles"
                    hx-trigger="click"
                    hx-target="#articles-with-pagination"
                    hx-include="#score-filter, #topic-filter, #feed-filter, #sort-filter"
                    hx-vals='{"liked": "%s"}'
                    hx-swap-oob="true">
                ★ Liked
            </button>`, activeClass, nextValue)

	if _, err := w.Write([]byte(buttonHTML)); err != nil {
		log.Printf("[ERROR] failed to write liked button: %v", err)
	}
}

// writePaginationControls renders pagination using the pagination template
func (s *Server) writePaginationControls(w http.ResponseWriter, req articlesPageRequest) {
	// create template data matching the structure used by full page render
	paginationData := struct {
		Articles      []domain.ItemWithClassification
		TotalCount    int
		MinScore      float64
		SelectedTopic string
		SelectedFeed  string
		SelectedSort  string
		ShowLikedOnly bool
		CurrentPage   int
		TotalPages    int
		PageNumbers   []int
		HasNext       bool
		HasPrev       bool
		IsHTMX        bool
	}{
		Articles:      req.articles,
		TotalCount:    req.totalCount,
		MinScore:      req.minScore,
		SelectedTopic: req.selectedTopic,
		SelectedFeed:  req.selectedFeed,
		SelectedSort:  req.selectedSort,
		ShowLikedOnly: req.showLikedOnly,
		CurrentPage:   req.currentPage,
		TotalPages:    req.totalPages,
		PageNumbers:   req.pageNumbers,
		HasNext:       req.hasNext,
		HasPrev:       req.hasPrev,
		IsHTMX:        true,
	}

	// execute the pagination template
	if err := s.templates.ExecuteTemplate(w, "pagination", paginationData); err != nil {
		log.Printf("[ERROR] failed to render pagination: %v", err)
	}
}

// addTopicHandler handles adding a new topic preference
func (s *Server) addTopicHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	topic := strings.TrimSpace(r.FormValue("topic"))
	topicType := r.FormValue("type") // "preferred" or "avoided"

	if topic == "" || (topicType != "preferred" && topicType != "avoided") {
		http.Error(w, "Invalid topic or type", http.StatusBadRequest)
		return
	}

	// get current topics
	settingKey := domain.SettingPreferredTopics
	if topicType == "avoided" {
		settingKey = domain.SettingAvoidedTopics
	}

	currentValue, err := s.db.GetSetting(ctx, settingKey)
	if err != nil {
		log.Printf("[ERROR] failed to get setting %s: %v", settingKey, err)
		http.Error(w, "Failed to get topics", http.StatusInternalServerError)
		return
	}

	// parse current topics
	var topics []string
	if currentValue != "" {
		if err := json.Unmarshal([]byte(currentValue), &topics); err != nil {
			log.Printf("[ERROR] failed to parse topics: %v", err)
			topics = []string{}
		}
	}

	// check if topic already exists
	for _, t := range topics {
		if strings.EqualFold(t, topic) {
			// topic already exists, just return current list
			s.renderTopicsList(w, topics, topicType)
			return
		}
	}

	// add new topic
	topics = append(topics, topic)

	// save updated topics
	updatedValue, err := json.Marshal(topics)
	if err != nil {
		log.Printf("[ERROR] failed to marshal topics: %v", err)
		http.Error(w, "Failed to save topics", http.StatusInternalServerError)
		return
	}

	if err := s.db.SetSetting(ctx, settingKey, string(updatedValue)); err != nil {
		log.Printf("[ERROR] failed to save setting %s: %v", settingKey, err)
		http.Error(w, "Failed to save topics", http.StatusInternalServerError)
		return
	}

	// render updated topics list
	s.renderTopicsList(w, topics, topicType)
}

// deleteTopicHandler handles removing a topic preference
func (s *Server) deleteTopicHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	topicToDelete := r.PathValue("topic")
	topicType := r.URL.Query().Get("type") // "preferred" or "avoided"

	if topicToDelete == "" || (topicType != "preferred" && topicType != "avoided") {
		http.Error(w, "Invalid topic or type", http.StatusBadRequest)
		return
	}

	// get current topics
	settingKey := domain.SettingPreferredTopics
	if topicType == "avoided" {
		settingKey = domain.SettingAvoidedTopics
	}

	currentValue, err := s.db.GetSetting(ctx, settingKey)
	if err != nil {
		log.Printf("[ERROR] failed to get setting %s: %v", settingKey, err)
		http.Error(w, "Failed to get topics", http.StatusInternalServerError)
		return
	}

	// parse current topics
	var topics []string
	if currentValue != "" {
		if err := json.Unmarshal([]byte(currentValue), &topics); err != nil {
			log.Printf("[ERROR] failed to parse topics: %v", err)
			http.Error(w, "Failed to parse topics", http.StatusInternalServerError)
			return
		}
	}

	// remove topic
	var updatedTopics []string
	for _, t := range topics {
		if !strings.EqualFold(t, topicToDelete) {
			updatedTopics = append(updatedTopics, t)
		}
	}

	// save updated topics
	updatedValue, err := json.Marshal(updatedTopics)
	if err != nil {
		log.Printf("[ERROR] failed to marshal topics: %v", err)
		http.Error(w, "Failed to save topics", http.StatusInternalServerError)
		return
	}

	if err := s.db.SetSetting(ctx, settingKey, string(updatedValue)); err != nil {
		log.Printf("[ERROR] failed to save setting %s: %v", settingKey, err)
		http.Error(w, "Failed to save topics", http.StatusInternalServerError)
		return
	}

	// render updated topics list
	s.renderTopicsList(w, updatedTopics, topicType)
}

// renderTopicsList renders the topics list HTML using template
func (s *Server) renderTopicsList(w http.ResponseWriter, topics []string, topicType string) {
	data := struct {
		Topics    []string
		TopicType string
		IsAvoided bool
	}{
		Topics:    topics,
		TopicType: topicType,
		IsAvoided: topicType == "avoided",
	}

	// use the pre-loaded template
	if err := s.templates.ExecuteTemplate(w, "topic-tags.html", data); err != nil {
		log.Printf("[ERROR] failed to render topics list: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
