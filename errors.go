package layerhop

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// APIError is returned for every non-2xx response. Code is a stable error code such as
// QUOTE_EXPIRED or NO_ROUTE (https://docs.layerhop.com/errors/<Code>).
type APIError struct {
	Code      string
	Message   string
	Status    int
	RequestID string
}

func (e *APIError) Error() string {
	s := fmt.Sprintf("layerhop: %s (%d): %s", e.Code, e.Status, e.Message)
	if e.RequestID != "" {
		s += " [requestId " + e.RequestID + "]"
	}
	return s
}

// DocsURL is the error catalog page for this code.
func (e *APIError) DocsURL() string { return "https://docs.layerhop.com/errors/" + e.Code }

func decodeAPIError(res *http.Response, raw []byte) error {
	var body struct {
		Error *struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"requestId"`
		} `json:"error"`
	}
	e := &APIError{Code: "INTERNAL", Message: http.StatusText(res.StatusCode), Status: res.StatusCode, RequestID: res.Header.Get("x-request-id")}
	if json.Unmarshal(raw, &body) == nil && body.Error != nil {
		if body.Error.Code != "" {
			e.Code = body.Error.Code
		}
		if body.Error.Message != "" {
			e.Message = body.Error.Message
		}
		if body.Error.RequestID != "" {
			e.RequestID = body.Error.RequestID
		}
	}
	return e
}
