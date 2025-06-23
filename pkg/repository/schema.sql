-- Simplified schema for LLM-based classification

-- Feed sources
CREATE TABLE IF NOT EXISTS feeds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    title TEXT DEFAULT '',
    description TEXT DEFAULT '',
    last_fetched DATETIME,
    next_fetch DATETIME,
    fetch_interval INTEGER DEFAULT 1800, -- 30 minutes
    error_count INTEGER DEFAULT 0,
    last_error TEXT DEFAULT '',
    enabled BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Articles with LLM classification
CREATE TABLE IF NOT EXISTS items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_id INTEGER NOT NULL,
    guid TEXT NOT NULL,
    title TEXT NOT NULL,
    link TEXT NOT NULL,
    description TEXT DEFAULT '',
    content TEXT DEFAULT '',        -- Original RSS content
    author TEXT DEFAULT '',
    published DATETIME,
    
    -- Extracted content
    extracted_content TEXT DEFAULT '',   -- Full article text
    extracted_rich_content TEXT DEFAULT '',  -- HTML formatted content
    extracted_at DATETIME,
    extraction_error TEXT DEFAULT '',
    
    -- LLM classification results
    relevance_score REAL DEFAULT 0,     -- 0-10 score from LLM
    explanation TEXT DEFAULT '',         -- Why this score
    topics JSON DEFAULT '[]',             -- Detected topics/tags
    summary TEXT DEFAULT '',             -- Article summary
    classified_at DATETIME,
    
    -- User feedback
    user_feedback TEXT DEFAULT '',      -- 'like', 'dislike', 'spam', empty
    feedback_at DATETIME,
    
    -- Metadata
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(feed_id, guid),
    FOREIGN KEY (feed_id) REFERENCES feeds(id) ON DELETE CASCADE
);

-- User preferences and settings
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_items_published ON items(published DESC);
CREATE INDEX IF NOT EXISTS idx_items_score ON items(relevance_score DESC);
CREATE INDEX IF NOT EXISTS idx_items_feedback ON items(user_feedback, feedback_at DESC);
CREATE INDEX IF NOT EXISTS idx_feeds_next ON feeds(next_fetch);

-- Additional performance indexes
CREATE INDEX IF NOT EXISTS idx_items_feed_published ON items(feed_id, published DESC);
CREATE INDEX IF NOT EXISTS idx_items_classification ON items(classified_at, relevance_score DESC);
CREATE INDEX IF NOT EXISTS idx_items_extraction ON items(extracted_at);
CREATE INDEX IF NOT EXISTS idx_items_score_feedback ON items(relevance_score DESC) WHERE user_feedback = '';
CREATE INDEX IF NOT EXISTS idx_feeds_enabled_next ON feeds(enabled, next_fetch) WHERE enabled = 1;

-- Topic-related indexes for JSON column
CREATE INDEX IF NOT EXISTS idx_items_topics_json ON items(json_extract(topics, '$'));
CREATE INDEX IF NOT EXISTS idx_items_score_classified ON items(relevance_score DESC, classified_at) WHERE classified_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_items_classified_score ON items(classified_at, relevance_score DESC) WHERE classified_at IS NOT NULL;

-- Update timestamp trigger
CREATE TRIGGER IF NOT EXISTS items_updated_at AFTER UPDATE ON items BEGIN
    UPDATE items SET updated_at = CURRENT_TIMESTAMP WHERE id = new.id;
END;

CREATE TRIGGER IF NOT EXISTS settings_updated_at AFTER UPDATE ON settings BEGIN
    UPDATE settings SET updated_at = CURRENT_TIMESTAMP WHERE key = new.key;
END;