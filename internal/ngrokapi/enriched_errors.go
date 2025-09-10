package ngrokapi

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/ngrok/ngrok-api-go/v7"
)

// whitespaceRegex is compiled once to avoid repeated compilation
var whitespaceRegex = regexp.MustCompile(`\s+`)

// Enriched Errors utilities to help interact with ngrok.Error and ngrok.IsErrorCode() returned by the ngrok API
// For right now these are all manually defined

type EnrichedError struct {
	name       string
	Code       int
	statusCode int32
}

// list of supported ngrok error codes
var (
	NgrokOpErrInternalServerError           = EnrichedError{"ERR_NGROK_20000", 20000, http.StatusInternalServerError}
	NgrokOpErrConfigurationError            = EnrichedError{"ERR_NGROK_20001", 20001, http.StatusBadRequest}
	NgrokOpErrFailedToCreateUpstreamService = EnrichedError{"ERR_NGROK_20002", 20002, http.StatusServiceUnavailable}
	NgrokOpErrFailedToCreateTargetService   = EnrichedError{"ERR_NGROK_20003", 20003, http.StatusServiceUnavailable}
	NgrokOpErrFailedToConnectServices       = EnrichedError{"ERR_NGROK_20004", 20004, http.StatusServiceUnavailable}
	NgrokOpErrEndpointDenied                = EnrichedError{"ERR_NGROK_20005", 20005, http.StatusForbidden}
	NgrokOpErrFailedToCreateCSR             = EnrichedError{"ERR_NGROK_20006", 20006, http.StatusInternalServerError}
)

func NewNgrokError(origErr error, ee EnrichedError, msg string) *ngrok.Error {
	if ngrokErr, ok := origErr.(*ngrok.Error); ok {
		// already have an ngrok.Error
		// overwrite the message and return
		return &ngrok.Error{
			Msg:     ngrokErr.Msg,
			Details: ngrokErr.Details,
		}
	}

	return &ngrok.Error{
		Msg:        fmt.Sprintf("%s: %s", msg, origErr),
		ErrorCode:  ee.name,
		StatusCode: ee.statusCode,
	}
}

// SanitizeErrorMessage cleans up ngrok API error messages for better display in kubectl output
func SanitizeErrorMessage(msg string) string {
	// Replace all whitespace (including line endings) with single spaces
	cleaned := whitespaceRegex.ReplaceAllString(msg, " ")
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
}

// IsTrafficPolicyError checks if an error message indicates a traffic policy configuration issue
func IsTrafficPolicyError(errMsg string) bool {
	return strings.Contains(errMsg, "policy") || strings.Contains(errMsg, "ERR_NGROK_2201")
}
