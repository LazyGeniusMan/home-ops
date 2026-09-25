package attach

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
)

// BodyLimit caps the request body (over-cap → 431). Call at the top of the
// notify handler; bodies are not required when attachments are present.
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

// ParseMultipart parses a multipart form: maxMemory bytes stay in RAM, the
// rest spills to disk. Incomings preserve arrival order; the caller defers
// r.MultipartForm.RemoveAll.
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
