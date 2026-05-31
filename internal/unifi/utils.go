package unifi

import (
	"strings"
)

// FormatURL substitutes successive %s placeholders in path with the values in
// params. Extra params beyond the placeholder count are appended verbatim to
// the last segment, matching the prior behaviour without panicking.
func FormatURL(path string, params ...string) string {
	segments := strings.Split(path, "%s")
	for i, param := range params {
		if param == "" {
			continue
		}
		idx := min(i, len(segments)-1)
		segments[idx] += param
	}

	return strings.Join(segments, "")
}
