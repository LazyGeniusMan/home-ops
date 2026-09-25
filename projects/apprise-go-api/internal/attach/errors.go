package attach

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// userinfoRe matches URL userinfo so stored errors never carry credentials.
var userinfoRe = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/@\s?#]+@`)

// redactURL strips URL userinfo (user[:pass]@ → ***@).
func redactURL(s string) string {
	return userinfoRe.ReplaceAllString(s, `${1}***@`)
}

// HTTP status codes for the attachment and notify paths.
const (
	// StatusBadRequest: malformed attachments, SSRF denials, oversized
	// staged files, disabled-attachment misuse.
	StatusBadRequest = 400
	// StatusFailedDependency: staged attachments a target cannot deliver.
	StatusFailedDependency = 424
	// StatusFieldsTooLarge: request body over APPRISE_UPLOAD_MAX_MEMORY_SIZE.
	StatusFieldsTooLarge = 431
)

// StatusError is a staging failure carrying its HTTP status. All staging
// failures are 400 except the body-cap overflow (431).
type StatusError struct {
	// Code is the HTTP status for this failure.
	Code int
	// Msg is the human-readable reason.
	Msg string
	// Err is the underlying cause, if any.
	Err error
}

// Error implements error.
func (e *StatusError) Error() string {
	if e == nil {
		return "attach: <nil>"
	}
	if e.Err != nil {
		return e.Msg + ": " + e.Err.Error()
	}
	return e.Msg
}

// Unwrap returns the underlying cause.
func (e *StatusError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// StatusCode reports the HTTP status for this failure.
func (e *StatusError) StatusCode() int {
	if e == nil {
		return 500
	}
	return e.Code
}

// Disabled reports attachments-disabled misuse (APPRISE_ATTACH_SIZE <= 0
// with attachment content present).
func Disabled() error {
	return &StatusError{Code: StatusBadRequest, Msg: "attach: attachment support has been disabled"}
}

// TooMany reports an over-limit attachment count.
func TooMany(got, max int) error {
	return &StatusError{
		Code: StatusBadRequest,
		Msg:  fmt.Sprintf("attach: there is a maximum of %d attachments (got %d)", max, got),
	}
}

// TooLargeFile reports a staged file exceeding APPRISE_ATTACH_SIZE.
func TooLargeFile(name string, limitBytes int64) error {
	return &StatusError{
		Code: StatusBadRequest,
		Msg:  fmt.Sprintf("attach: attachment %q exceeds the %d byte limit", name, limitBytes),
	}
}

// BadAttachment reports a malformed attachment entry.
func BadAttachment(format string, args ...any) error {
	return &StatusError{Code: StatusBadRequest, Msg: fmt.Sprintf("attach: "+format, args...)}
}

// Denied reports an SSRF-policy rejection. The URL is stored redacted
// (userinfo stripped) so logs never carry secrets.
func Denied(rawURL string) error {
	return &StatusError{Code: StatusBadRequest, Msg: fmt.Sprintf("attach: denied attachment (blocked web request): %s", redactURL(rawURL))}
}

// FetchFailed reports a remote attachment download failure. The URL is
// stored redacted; wrap with %w (never %v) so errors.Is/As see the cause.
func FetchFailed(rawURL string, err error) error {
	return &StatusError{Code: StatusBadRequest, Msg: fmt.Sprintf("attach: failed to retrieve attachment: %s", redactURL(rawURL)), Err: err}
}

// BodyTooLarge reports a request body exceeding
// APPRISE_UPLOAD_MAX_MEMORY_SIZE.
func BodyTooLarge(limitBytes int64) error {
	return &StatusError{
		Code: StatusFieldsTooLarge,
		Msg:  fmt.Sprintf("attach: request body exceeds the %d byte limit", limitBytes),
	}
}

// unsupportedMarker matches apprise-go's unexported
// ErrAttachmentsUnsupported ("attachments unsupported by target").
const unsupportedMarker = "attachments unsupported by target"

// IsUnsupportedAttachments reports whether err (possibly wrapped, e.g. in
// apprise-go TargetError values joined by Send) indicates a target without
// attachment support.
func IsUnsupportedAttachments(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), unsupportedMarker)
}

// SendFailure is a delivery failure for staged attachments, carrying the
// target URL and filename so failures are never silent. It maps to HTTP 424.
type SendFailure struct {
	// Target is the notification URL that failed.
	Target string
	// Filename names the staged attachment involved (may be empty when the
	// failure is not tied to a single file).
	Filename string
	// Err is the underlying send error.
	Err error
}

// Error implements error.
func (e *SendFailure) Error() string {
	if e == nil {
		return "attach: <nil send failure>"
	}
	if e.Filename != "" {
		return fmt.Sprintf("attach: send to %s with attachment %q failed: %v", e.Target, e.Filename, e.Err)
	}
	return fmt.Sprintf("attach: send to %s failed: %v", e.Target, e.Err)
}

// Unwrap returns the underlying send error.
func (e *SendFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// StatusCode reports HTTP 424 for send failures.
func (e *SendFailure) StatusCode() int { return StatusFailedDependency }

// WrapSendError wraps a target send error with attachment context for the
// 424 path.
func WrapSendError(target, filename string, err error) *SendFailure {
	return &SendFailure{Target: target, Filename: filename, Err: err}
}

// StatusCodeOf maps staging and send errors to HTTP statuses: 400 for bad
// attachments/denials, 431 for body-cap overflow, 424 for send failures,
// 500 for anything else.
func StatusCodeOf(err error) int {
	if err == nil {
		return 200
	}
	var se *StatusError
	if errors.As(err, &se) {
		return se.StatusCode()
	}
	var sf *SendFailure
	if errors.As(err, &sf) {
		return sf.StatusCode()
	}
	return 500
}
