package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopics_Value(t *testing.T) {
	tests := []struct {
		name     string
		topics   Topics
		expected string
	}{
		{
			name:     "empty topics",
			topics:   Topics{},
			expected: "[]",
		},
		{
			name:     "nil topics",
			topics:   nil,
			expected: "[]",
		},
		{
			name:     "single topic",
			topics:   Topics{"golang"},
			expected: `["golang"]`,
		},
		{
			name:     "multiple topics",
			topics:   Topics{"golang", "backend", "microservices"},
			expected: `["golang","backend","microservices"]`,
		},
		{
			name:     "topics with special characters",
			topics:   Topics{"c++", "c#", "machine-learning"},
			expected: `["c++","c#","machine-learning"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := tt.topics.Value()
			require.NoError(t, err)

			// convert to string for comparison
			switch v := value.(type) {
			case []byte:
				assert.JSONEq(t, tt.expected, string(v))
			case string:
				assert.JSONEq(t, tt.expected, v)
			default:
				t.Fatalf("unexpected type: %T", value)
			}
		})
	}
}

func TestTopics_Scan(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected Topics
		wantErr  bool
	}{
		{
			name:     "scan from string",
			input:    `["golang", "backend"]`,
			expected: Topics{"golang", "backend"},
		},
		{
			name:     "scan from bytes",
			input:    []byte(`["rust", "systems"]`),
			expected: Topics{"rust", "systems"},
		},
		{
			name:     "scan empty array",
			input:    `[]`,
			expected: Topics{},
		},
		{
			name:     "scan nil value",
			input:    nil,
			expected: Topics{},
		},
		{
			name:     "scan invalid json",
			input:    `["invalid"`,
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "scan non-array json",
			input:    `{"not": "array"}`,
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "scan from unsupported type",
			input:    123,
			expected: Topics{},
		},
		{
			name:     "scan single topic",
			input:    `["kubernetes"]`,
			expected: Topics{"kubernetes"},
		},
		{
			name:     "scan topics with unicode",
			input:    `["münchen", "技术", "café"]`,
			expected: Topics{"münchen", "技术", "café"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var topics Topics
			err := topics.Scan(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, topics)
		})
	}
}

func TestTopics_RoundTrip(t *testing.T) {
	// test that Value and Scan are inverse operations
	tests := []struct {
		name   string
		topics Topics
	}{
		{
			name:   "empty topics",
			topics: Topics{},
		},
		{
			name:   "single topic",
			topics: Topics{"docker"},
		},
		{
			name:   "multiple topics",
			topics: Topics{"ai", "ml", "deep-learning", "neural-networks"},
		},
		{
			name:   "topics with special chars and unicode",
			topics: Topics{"c++", "f#", "объект", "データ"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// convert to database value
			value, err := tt.topics.Value()
			require.NoError(t, err)

			// scan back
			var scanned Topics
			err = scanned.Scan(value)
			require.NoError(t, err)

			// should be equal
			assert.Equal(t, tt.topics, scanned)
		})
	}
}
