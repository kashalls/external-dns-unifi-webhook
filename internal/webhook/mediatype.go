package webhook

import (
	"errors"
	"fmt"
	"strings"
)

const mediaTypeFormat = "application/external.dns.webhook+json;"

// supportedMediaVersions enumerates the webhook protocol versions this server
// understands. The first entry is also the version used for outgoing
// Content-Type headers.
var supportedMediaVersions = []string{"1"}

var errUnsupportedMediaType = errors.New("unsupported media type version")

var mediaTypeVersion1 = mediaTypeVersion("1")

type mediaType string

func mediaTypeVersion(v string) mediaType {
	return mediaType(mediaTypeFormat + "version=" + v)
}

func (m mediaType) Is(headerValue string) bool {
	return string(m) == headerValue
}

// checkAndGetMediaTypeHeaderValue returns the matched version when value
// corresponds to one of the supported media types, or an error listing the
// supported types.
func checkAndGetMediaTypeHeaderValue(value string) (string, error) {
	for _, v := range supportedMediaVersions {
		if mediaTypeVersion(v).Is(value) {
			return v, nil
		}
	}

	supported := make([]string, len(supportedMediaVersions))
	for i, v := range supportedMediaVersions {
		supported[i] = string(mediaTypeVersion(v))
	}

	return "", fmt.Errorf("%w: received %q, supported media types are: %s",
		errUnsupportedMediaType, value, strings.Join(supported, ", "))
}
