package unifi

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kashalls/external-dns-provider-unifi/cmd/webhook/init/log"
	"go.uber.org/zap"
)

// FormatUrl formats a URL with the given parameters.
func FormatUrl(path string, params ...string) string {
	segments := strings.Split(path, "%s")
	for i, param := range params {
		if param != "" {
			segments[i] += param
		}
	}
	return strings.Join(segments, "")
}

// Copied from external-dns pr from starcraft66's cloudflare support.
// Thank you, starcraft66.
func ParseSRVContent(target string) (*SRVData, error) {
	parts := strings.Fields(target)
	if len(parts) != 4 {
		return nil, fmt.Errorf("srv record contains invalid content: %s", target)
	}

	priority, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("srv record contains invalid priority: %s", parts[0])
	}

	weight, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("srv record contains invalid weight: %s", parts[1])
	}

	port, err := strconv.Atoi(parts[2])
	if err != nil {
		return nil, fmt.Errorf("srv record contains invalid port: %s", parts[2])
	}

	log.With(
		zap.Any("priority", priority),
		zap.Any("weight", weight),
		zap.Any("port", port),
		zap.Any("target", parts[3]),
	).Debug("parsed srv content")

	// Target can be an IP or a hostname (which is essentially a string)
	return &SRVData{
		Priority: priority,
		Weight:   weight,
		Port:     port,
		Target:   parts[3],
	}, nil

}
