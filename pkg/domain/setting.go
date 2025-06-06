package domain

import "time"

// Setting represents a key-value configuration setting
type Setting struct {
	Key       string
	Value     string
	UpdatedAt time.Time
}

// Setting keys
const (
	SettingPreferredTopics = "preferred_topics"
	SettingAvoidedTopics   = "avoided_topics"
)
