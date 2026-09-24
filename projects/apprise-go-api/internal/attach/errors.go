package attach

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// userinfoRe matches URL userinfo (user[:pass]@) so stored error messages
// never carry credentials embedded in attachment URLs.
var userinfoRe = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/@\s?#]+@`)

// redactURL strips URL userinfo (user[:pass]@ → ***@). Non-URL text passes
// through unchanged.
func redactURL(s string) string {
	return userinfoRe.ReplaceAllString(s, `${1}***@`)
}

// HTTP status codes mirroring the Python apprise-api ResponseCode values
// used by the attachment and notify paths.
const (
	// StatusBadRequest is returned for malformed attachments, SSRF denials,
	// oversized staged files, and disabled-attachment misuse.
	StatusBadRequest = 400
	// StatusFailedDependency is returned when staged attachments cannot be
	// delivered (e.g. a target without attachment support).
	StatusFailedDependency = 424
	// StatusFieldsTooLarge is returned when the request body exceeds
	// APPRISE_UPLOAD_MAX_MEMORY_SIZE.
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
// with attachment content present). Python: "Attachment support has been
// disabled" -> 400.
func Disabled() error {
	return &StatusError{Code: StatusBadRequest, Msg: "attach: attachment support has been disabled"}
}

// TooMany reports an over-limit attachment count. Python: "There is a
// maximum of N attachments" -> 400.
func TooMany(got, max int) error {
	return &StatusError{
		Code: StatusBadRequest,
		Msg:  fmt.Sprintf("attach: there is a maximum of %d attachments (got %d)", max, got),
	}
}

// TooLargeFile reports a staged file exceeding the per-file limit
// (APPRISE_ATTACH_SIZE). Python maps this to ValueError -> 400 (431 is
// reserved for the request body/memory cap).
func TooLargeFile(name string, limitBytes int64) error {
	return &StatusError{
		Code: StatusBadRequest,
		Msg:  fmt.Sprintf("attach: attachment %q exceeds the %d byte limit", name, limitBytes),
	}
}

// BadAttachment reports a malformed attachment entry: bad filename, bad
// base64, non-URL string, dict without base64/url, or undeliverable type.
// Python: ValueError -> 400.
func BadAttachment(format string, args ...any) error {
	return &StatusError{Code: StatusBadRequest, Msg: fmt.Sprintf("attach: "+format, args...)}
}

// Denied reports an SSRF-policy rejection of a remote attachment URL.
// Python: ValueError (blocked web request) -> 400. The URL is stored
// redacted in Msg (userinfo stripped) so logs and %-suffixes never carry
// secrets; the domain stays so operators can diagnose policy misses.
func Denied(rawURL string) error {
	return &StatusError{Code: StatusBadRequest, Msg: fmt.Sprintf("attach: denied attachment (blocked web request): %s", redactURL(rawURL))}
}

// FetchFailed reports a remote attachment download failure (network error
// or non-2xx status). Python: ValueError (failed to retrieve) -> 400. The
// URL is stored redacted; wrap with %w (never %v) so errors.Is/As see the
// cause.
func FetchFailed(rawURL string, err error) error {
	return &StatusError{Code: StatusBadRequest, Msg: fmt.Sprintf("attach: failed to retrieve attachment: %s", redactURL(rawURL)), Err: err}
}

// BodyTooLarge reports a request body exceeding
// APPRISE_UPLOAD_MAX_MEMORY_SIZE. Python: RequestDataTooBig -> 431.
func BodyTooLarge(limitBytes int64) error {
	return &StatusError{
		Code: StatusFieldsTooLarge,
		Msg:  fmt.Sprintf("attach: request body exceeds the %d byte limit", limitBytes),
	}
}

// unsupportedMarker matches apprise-go's internal
// notify.ErrAttachmentsUnsupported ("attachments unsupported by target"),
// which is not exported outside its module. Matching on the message keeps
// the API layer decoupled while still surfacing the exact condition.
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
