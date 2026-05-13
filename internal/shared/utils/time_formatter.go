package utils

import "time"

// FormatTimeRFC3339 formats a time.Time to RFC3339 string
func FormatTimeRFC3339(t time.Time) string {
	return t.Format(time.RFC3339)
}

// ParseTimeRFC3339 parses an RFC3339 string to time.Time
func ParseTimeRFC3339(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}
