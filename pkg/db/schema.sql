-- feeds table stores RSS feed sources
CREATE TABLE IF NOT EXISTS feeds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    description TEXT,
    last_fetched DATETIME,
    next_fetch DATETIME,
    fetch_interval INTEGER DEFAULT 3600, -- seconds, default 1 hour
    last_error TEXT,
    error_count INTEGER DEFAULT 0,
    enabled BOOLEAN DEFAULT 1,
    priority INTEGER DEFAULT 0,
    avg_score REAL, -- historical average score
    metadata JSON, -- feed-specific settings
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- items table stores individual feed items/articles
CREATE TABLE IF NOT EXISTS items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_id INTEGER NOT NULL,
    guid TEXT NOT NULL,
    title TEXT NOT NULL,
    link TEXT NOT NULL,
    description TEXT,
    content TEXT, -- plain text version
    content_html TEXT, -- HTML version for structure
    content_hash TEXT, -- for deduplication
    published DATETIME,
    author TEXT,
    language TEXT, -- extracted by trafilatura
    read_time INTEGER, -- estimated minutes
    media_count INTEGER,
    extraction_method TEXT, -- trafilatura, fallback, etc
    extraction_mode TEXT, -- precision, balanced, recall
    content_extracted BOOLEAN DEFAULT 0,
    fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (feed_id) REFERENCES feeds(id) ON DELETE CASCADE,
    UNIQUE(feed_id, guid)
);

-- content table stores extracted full content
CREATE TABLE IF NOT EXISTS content (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id INTEGER NOT NULL UNIQUE,
    full_content TEXT NOT NULL,
    extracted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    extraction_error TEXT,
    FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE
);

-- categories table for classification rules
CREATE TABLE IF NOT EXISTS categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    keywords JSON NOT NULL, -- array of keywords
    is_positive BOOLEAN NOT NULL DEFAULT 1,
    weight REAL DEFAULT 1.0,
    parent_id INTEGER,
    active BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (parent_id) REFERENCES categories(id) ON DELETE SET NULL
);

-- junction table for item categories (many-to-many)
CREATE TABLE IF NOT EXISTS item_categories (
    item_id INTEGER NOT NULL,
    category_id INTEGER NOT NULL,
    confidence REAL DEFAULT 1.0,
    PRIMARY KEY (item_id, category_id),
    FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE,
    FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE CASCADE
);

-- article scores from classification
CREATE TABLE IF NOT EXISTS article_scores (
    article_id INTEGER NOT NULL,
    rule_score REAL,
    ml_score REAL,
    source_score REAL,
    recency_score REAL,
    final_score REAL NOT NULL,
    explanation JSON, -- why this score
    scored_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    model_version INTEGER,
    PRIMARY KEY (article_id),
    FOREIGN KEY (article_id) REFERENCES items(id) ON DELETE CASCADE
);

-- user feedback for ML training
CREATE TABLE IF NOT EXISTS user_feedback (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    article_id INTEGER NOT NULL,
    feedback_type TEXT NOT NULL, -- 'interesting', 'boring', 'spam'
    feedback_value INTEGER, -- 1-5 scale for nuanced feedback
    feedback_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    time_spent INTEGER, -- seconds spent reading
    used_for_training BOOLEAN DEFAULT 0,
    UNIQUE(article_id),
    FOREIGN KEY (article_id) REFERENCES items(id) ON DELETE CASCADE
);

-- user actions tracking
CREATE TABLE IF NOT EXISTS user_actions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    article_id INTEGER NOT NULL,
    action TEXT NOT NULL, -- 'view', 'click', 'share', 'save'
    action_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    context JSON,
    FOREIGN KEY (article_id) REFERENCES items(id) ON DELETE CASCADE
);

-- ML model storage
CREATE TABLE IF NOT EXISTS ml_models (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    model_type TEXT NOT NULL,
    model_data BLOB NOT NULL,
    feature_config JSON,
    training_stats JSON, -- precision, recall, f1
    sample_count INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    is_active BOOLEAN DEFAULT 0
);

-- system configuration
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value JSON NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_feeds_enabled ON feeds(enabled);
CREATE INDEX IF NOT EXISTS idx_feeds_next_fetch ON feeds(next_fetch);
CREATE INDEX IF NOT EXISTS idx_items_feed_id ON items(feed_id);
CREATE INDEX IF NOT EXISTS idx_items_published ON items(published DESC);
CREATE INDEX IF NOT EXISTS idx_items_feed_published ON items(feed_id, published DESC);
CREATE INDEX IF NOT EXISTS idx_items_content_extracted ON items(content_extracted);
CREATE INDEX IF NOT EXISTS idx_items_url_hash ON items(link, content_hash);
CREATE INDEX IF NOT EXISTS idx_scores_final ON article_scores(final_score DESC);
CREATE INDEX IF NOT EXISTS idx_feedback_training ON user_feedback(used_for_training, feedback_at);
CREATE INDEX IF NOT EXISTS idx_actions_article ON user_actions(article_id, action_at);
CREATE INDEX IF NOT EXISTS idx_categories_active ON categories(active);
CREATE INDEX IF NOT EXISTS idx_item_categories_item ON item_categories(item_id);
CREATE INDEX IF NOT EXISTS idx_item_categories_category ON item_categories(category_id);

-- Full-text search virtual table for items
CREATE VIRTUAL TABLE IF NOT EXISTS items_fts USING fts5(
    title, 
    description, 
    content
);

-- Triggers to keep FTS index up to date
CREATE TRIGGER IF NOT EXISTS items_fts_insert AFTER INSERT ON items BEGIN
    INSERT INTO items_fts(rowid, title, description, content) 
    VALUES (new.id, new.title, new.description, new.content);
END;

CREATE TRIGGER IF NOT EXISTS items_fts_delete AFTER DELETE ON items BEGIN
    DELETE FROM items_fts WHERE rowid = old.id;
END;

CREATE TRIGGER IF NOT EXISTS items_fts_update AFTER UPDATE ON items BEGIN
    DELETE FROM items_fts WHERE rowid = new.id;
    INSERT INTO items_fts(rowid, title, description, content) 
    VALUES (new.id, new.title, new.description, new.content);
END;

-- Update timestamp triggers
CREATE TRIGGER IF NOT EXISTS feeds_updated_at AFTER UPDATE ON feeds BEGIN
    UPDATE feeds SET updated_at = CURRENT_TIMESTAMP WHERE id = new.id;
END;

CREATE TRIGGER IF NOT EXISTS items_updated_at AFTER UPDATE ON items BEGIN
    UPDATE items SET updated_at = CURRENT_TIMESTAMP WHERE id = new.id;
END;

CREATE TRIGGER IF NOT EXISTS categories_updated_at AFTER UPDATE ON categories BEGIN
    UPDATE categories SET updated_at = CURRENT_TIMESTAMP WHERE id = new.id;
END;

CREATE TRIGGER IF NOT EXISTS settings_updated_at AFTER UPDATE ON settings BEGIN
    UPDATE settings SET updated_at = CURRENT_TIMESTAMP WHERE key = new.key;
END;