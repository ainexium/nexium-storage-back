package apierr

import "fmt"

type APIError struct {
	Code    int    `json:"-"`
	Message string `json:"message"`
}

func (e *APIError) Error() string { return e.Message }

func New(code int, msg string) *APIError {
	return &APIError{Code: code, Message: msg}
}

func Newf(code int, format string, args ...any) *APIError {
	return &APIError{Code: code, Message: fmt.Sprintf(format, args...)}
}

var (
	ErrUnauthorized    = New(401, "unauthorized")
	ErrForbidden       = New(403, "forbidden")
	ErrNotFound        = New(404, "not found")
	ErrTooManyRequests = New(429, "too many requests — slow down")
	ErrInternal        = New(500, "internal server error")
)

func ErrBadRequest(msg string) *APIError  { return New(400, msg) }
func ErrConflict(msg string) *APIError    { return New(409, msg) }
func ErrUnprocessable(msg string) *APIError { return New(422, msg) }
