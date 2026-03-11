package models

import (
	"net/http"
	"strconv"
)

var (
	ErrCodeBadRequest          = strconv.Itoa(http.StatusBadRequest)
	ErrCodeForbidden           = strconv.Itoa(http.StatusForbidden)
	ErrCodeNotFound            = strconv.Itoa(http.StatusNotFound)
	ErrCodeInternalServerError = strconv.Itoa(http.StatusInternalServerError)
)

// NexErrorI Must satisfy https://github.com/ConnectEverything/control-plane/blob/aebd477e134f0b0473ee7b9490ac0cafced4b31d/go/cp-server/server/helpers.go#L47
// of which NexErrorI is a local copy of
type NexErrorI interface {
	ID() string
	Error() string
	Body() []byte
	Status() int
}

var (
	_ error     = (*NexError)(nil)
	_ NexErrorI = (*NexError)(nil)
)

type NexError struct {
	id          string
	err         error
	friendlyMsg string
	httpStatus  int
}

// ID returns the correlation ID for this error. When the error originates
// from the server, this matches the error_id in the server's logs.
func (ne *NexError) ID() string {
	return ne.id
}

// Error returns the raw internal error message. This may contain
// implementation details and should not be shown to end users — use
// Body for user-facing output.
func (ne *NexError) Error() string {
	return ne.err.Error()
}

// Body returns the user-safe friendly message suitable for display
// to end users or inclusion in API responses.
func (ne *NexError) Body() []byte {
	return []byte(ne.friendlyMsg)
}

// Status returns the HTTP status code associated with this error.
func (ne *NexError) Status() int {
	return ne.httpStatus
}

func NewNexError(id string, err error, friendlyMsg string, httpStatus int) *NexError {
	return &NexError{
		id:          id,
		err:         err,
		friendlyMsg: friendlyMsg,
		httpStatus:  httpStatus,
	}
}
