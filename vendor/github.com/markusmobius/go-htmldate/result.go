package htmldate

import "time"

var resultZero = Result{}

// Result is the result of date time extraction.
type Result struct {
	// DateTime is the extracted date time.
	DateTime time.Time
	// HasTime specifies whether the result contains time or not.
	HasTime bool
	// HasTimezone specifies whether the result contains timezone or not.
	// Useful for differentiating UTC timezone or timezone not found.
	HasTimezone bool
	// SrcString is the source where the date and time extracted.
	SrcString string
}

// IsZero reports whether the result is empty or not.
// Wrapper for `Result.DateTime.IsZero`.
func (r Result) IsZero() bool {
	return r.DateTime.IsZero()
}

// Format returns a textual representation of the time value formatted according to
// the specified layout. Wrapper for `Result.DateTime.Format`.
func (r Result) Format(layout string) string {
	return r.DateTime.Format(layout)
}
