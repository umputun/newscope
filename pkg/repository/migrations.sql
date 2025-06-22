-- Migrations for newscope database
-- These migrations are idempotent and safe to run multiple times

-- Migration 1: Add summary column to items table
-- Note: SQLite doesn't support IF NOT EXISTS for columns, so this may fail on repeated runs
-- The Go code will check if the column exists before running this
-- ALTER TABLE items ADD COLUMN summary TEXT DEFAULT '';

-- Migration 2: Add performance indexes
CREATE INDEX IF NOT EXISTS idx_items_feed_published ON items(feed_id, published DESC);
CREATE INDEX IF NOT EXISTS idx_items_classification ON items(classified_at, relevance_score DESC);
CREATE INDEX IF NOT EXISTS idx_items_extraction ON items(extracted_at);
CREATE INDEX IF NOT EXISTS idx_items_score_feedback ON items(relevance_score DESC) WHERE user_feedback = '';
CREATE INDEX IF NOT EXISTS idx_feeds_enabled_next ON feeds(enabled, next_fetch) WHERE enabled = 1;