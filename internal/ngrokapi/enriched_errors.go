package ngrokapi

import (
	"fmt"
	"net/http"

	"github.com/ngrok/ngrok-api-go/v7"
)

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
