package unifi

import (
	"strings"
)

// FormatURL formats a URL with the given parameters.
func FormatURL(path string, params ...string) string {
	segments := strings.Split(path, "%s")
	for i, param := range params {
		if param != "" {
			segments[i] += param
		}
	}

	return strings.Join(segments, "")
}
