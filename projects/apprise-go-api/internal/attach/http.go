package attach

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
)

// BodyLimit caps the request body via http.MaxBytesReader before parsing,
// enforcing APPRISE_UPLOAD_MAX_MEMORY_SIZE. Over-cap reads fail with a
// *http.MaxBytesError, which BodyTooLarge maps to the 431 StatusError.
// Call at the top of the notify handler; pass the same cap to
// ParseMultipart so the spill-to-disk budget matches. Bodies are not
// required when an attachment is present — call HasAttachment on the
// result to apply the body-required rule.
//
// The returned body is the (possibly capped) request body for downstream
// JSON decoding.
func BodyLimit(w http.ResponseWriter, r *http.Request, maxBytes int64) io.Reader {
	if maxBytes <= 0 {
		return r.Body
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	return r.Body
}

// IsBodyTooLarge reports whether err is a body-cap overflow (431 path),
// including wrapped *http.MaxBytesError values.
func IsBodyTooLarge(err error) bool {
	if err == nil {
		return false
	}
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return true
	}
	return strings.Contains(err.Error(), "request body too large")
}

// MapBodyError converts a body read/parse error to BodyTooLarge (431) when
// it is a cap overflow, or nil when err is nil. Non-cap errors pass
// through unchanged for the caller to map (usually 400).
func MapBodyError(err error, maxBytes int64) error {
	if err == nil {
		return nil
	}
	if IsBodyTooLarge(err) {
		return BodyTooLarge(maxBytes)
	}
	return err
}

// ParseMultipart parses a multipart form under the memory budget,
// streaming large parts to tempfile: maxMemory bytes stay in RAM, the rest
// spills to os.TempDir (Go runtime), not the attach dir. Staging under the
// per-file APPRISE_ATTACH_SIZE cap happens separately in stageStream, so a
// 200MB attach dir and a 3MB memory budget coexist: ParseMultipartForm
// spills to disk, and only the staged copy lands in the attach dir. A
// *http.MaxBytesError from an over-cap body maps to the 431 StatusError.
//
// Returned Incomings preserve arrival order across all field names,
// mirroring Python's request.FILES iteration. Spill files are removed by
// r.MultipartForm.RemoveAll, which the caller defers.
func ParseMultipart(r *http.Request, maxMemoryBytes int64) ([]Incoming, error) {
	if maxMemoryBytes <= 0 {
		return nil, BadAttachment("max memory bytes must be positive, got %d", maxMemoryBytes)
	}
	if err := r.ParseMultipartForm(maxMemoryBytes); err != nil {
		return nil, err
	}
	form := r.MultipartForm
	if form == nil || form.File == nil {
		return nil, nil
	}
	var out []Incoming
	for field, headers := range form.File {
		for _, fh := range headers {
			fh := fh
			out = append(out, Incoming{
				Field:       field,
				Filename:    fh.Filename,
				ContentType: contentTypeOf(fh.Header),
				Size:        fh.Size,
				Open: func() (io.ReadCloser, error) {
					return fh.Open()
				},
			})
		}
	}
	return out, nil
}

// contentTypeOf extracts the part Content-Type header value.
func contentTypeOf(h map[string][]string) string {
	ct := ""
	for k, vs := range h {
		if strings.EqualFold(k, "Content-Type") && len(vs) > 0 {
			ct = vs[0]
			break
		}
	}
	if ct == "" {
		return ""
	}
	if media, _, err := mime.ParseMediaType(ct); err == nil {
		return media
	}
	if i := strings.Index(ct, ";"); i >= 0 {
		return strings.TrimSpace(ct[:i])
	}
	return strings.TrimSpace(ct)
}

// FilePart wraps a *multipart.FileHeader as an Incoming for direct staging
// without going through ParseMultipart. Any field name is accepted.
func FilePart(field string, fh *multipart.FileHeader) Incoming {
	if fh == nil {
		return Incoming{Field: field}
	}
	return Incoming{
		Field:       field,
		Filename:    fh.Filename,
		ContentType: contentTypeOf(fh.Header),
		Size:        fh.Size,
		Open: func() (io.ReadCloser, error) {
			return fh.Open()
		},
	}
}

// FormURLs collects non-blank attachment URL strings for one alias key,
// mirroring Python's getlist filter. The alias priority is resolved by the
// caller.
func FormURLs(values []string) []string {
	var out []string
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}
