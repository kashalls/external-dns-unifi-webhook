package webhook

import (
	"strings"

	"github.com/cockroachdb/errors"
)

const (
	mediaTypeFormat        = "application/external.dns.webhook+json;"
	supportedMediaVersions = "1"
)

var errUnsupportedMediaType = errors.New("unsupported media type version")

var mediaTypeVersion1 = mediaTypeVersion("1")

type mediaType string

func mediaTypeVersion(v string) mediaType {
	return mediaType(mediaTypeFormat + "version=" + v)
}

func (m mediaType) Is(headerValue string) bool {
	return string(m) == headerValue
}

func checkAndGetMediaTypeHeaderValue(value string) (string, error) {
	for v := range strings.SplitSeq(supportedMediaVersions, ",") {
		if mediaTypeVersion(v).Is(value) {
			return v, nil
		}
	}

	supportedMediaTypesString := ""
	var supportedMediaTypesStringSb36 strings.Builder
	for i, v := range strings.Split(supportedMediaVersions, ",") {
		sep := ""
		if i < len(supportedMediaVersions)-1 {
			sep = ", "
		}
		supportedMediaTypesStringSb36.WriteString(string(mediaTypeVersion(v)) + sep)
	}
	supportedMediaTypesString += supportedMediaTypesStringSb36.String()

	return "", errors.Wrapf(errUnsupportedMediaType, "received '%s', supported media types are: '%s'", value, supportedMediaTypesString)
}
