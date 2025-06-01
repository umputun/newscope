package repository

import "strings"

// criticalError wraps an error to signal repeater to stop retrying
type criticalError struct {
	err error
}

func (e *criticalError) Error() string {
	return e.err.Error()
}

// isLockError checks if an error is a SQLite lock/busy error
func isLockError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "SQLITE_BUSY") ||
		strings.Contains(errStr, "database is locked") ||
		strings.Contains(errStr, "database table is locked")
}