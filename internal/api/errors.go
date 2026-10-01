package api

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Sentinels for errors.Is against a *StatusError. See
// https://docs.hardcover.app/api/getting-started/#api-response-codes
var (
	ErrBadRequest        = errors.New("bad request")                // 400
	ErrUnauthorized      = errors.New("unauthorized")               // 401
	ErrInsufficientScope = errors.New("insufficient scope")         // 403
	ErrUnsupportedOp     = errors.New("unsupported operation")      // 403
	ErrTopLevelLimit     = errors.New("too many top-level queries") // 403
	ErrOverCapacity      = errors.New("request exceeds capacity")   // 403
	ErrForbidden         = errors.New("forbidden")                  // 403
	ErrNotFound          = errors.New("not found")                  // 404
	ErrTimeout           = errors.New("server timeout")             // 408
	ErrRateLimited       = errors.New("rate limited")               // 429
	ErrUnavailable       = errors.New("service unavailable")        // 5xx
)

// Is lets callers match a StatusError with errors.Is. The specific 403
// variants also satisfy ErrForbidden.
func (e *StatusError) Is(target error) bool {
	switch target {
	case ErrBadRequest:
		return e.Code == http.StatusBadRequest
	case ErrUnauthorized:
		return e.Code == http.StatusUnauthorized
	case ErrInsufficientScope:
		return e.APIError == "insufficient_scope"
	case ErrUnsupportedOp:
		return e.APIError == "unsupported_operation"
	case ErrTopLevelLimit:
		return e.APIError == "top_level_limit_exceeded"
	case ErrOverCapacity:
		return e.APIError == "request_exceeds_capacity"
	case ErrForbidden:
		return e.Code == http.StatusForbidden
	case ErrNotFound:
		return e.Code == http.StatusNotFound
	case ErrTimeout:
		return e.Code == http.StatusRequestTimeout
	case ErrRateLimited:
		return e.Code == http.StatusTooManyRequests
	case ErrUnavailable:
		return e.Code >= http.StatusInternalServerError
	}
	return false
}

// apiErrorBody covers the error shapes the API documents: the flat
// {error, error_description|message, scope} form, and a GraphQL-style
// {errors: [{message, extensions: {code}}]} array.
type apiErrorBody struct {
	Error       string         `json:"error"`
	Description string         `json:"error_description"`
	Message     string         `json:"message"`
	Scope       string         `json:"scope"`
	Errors      []apiErrorItem `json:"errors"`
}

// apiErrorItem is an entry of an "errors" array: a bare code string
// ("request_exceeds_capacity") or {message, extensions: {code}}.
type apiErrorItem struct {
	Code    string
	Message string
}

func (i *apiErrorItem) UnmarshalJSON(data []byte) error {
	var code string
	if json.Unmarshal(data, &code) == nil {
		i.Code = code
		return nil
	}
	var obj struct {
		Message    string `json:"message"`
		Extensions struct {
			Code string `json:"code"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	i.Code, i.Message = obj.Extensions.Code, obj.Message
	return nil
}

// parseErrorBody fills the structured fields of e from a JSON response body.
// A body that isn't JSON (an edge proxy's HTML 503, say) leaves them empty.
func (e *StatusError) parseErrorBody(body []byte) {
	var b apiErrorBody
	if json.Unmarshal(body, &b) != nil {
		return
	}
	e.APIError = b.Error
	e.Description = b.Description
	if e.Description == "" {
		e.Description = b.Message
	}
	e.Scope = b.Scope
	if e.APIError == "" && len(b.Errors) > 0 {
		e.APIError = b.Errors[0].Code
		if e.Description == "" {
			e.Description = b.Errors[0].Message
		}
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
