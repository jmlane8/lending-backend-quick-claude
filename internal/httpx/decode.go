package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// MalformedError reports a request body that could not be read as the expected JSON.
// Reason is safe to return to the client: it names fields, never values.
type MalformedError struct{ Reason string }

func (e *MalformedError) Error() string { return "malformed request: " + e.Reason }

// ReadBody reads the whole request body, limited to maxBytes.
func ReadBody(w http.ResponseWriter, r *http.Request, maxBytes int64) ([]byte, error) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBytes))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return nil, &MalformedError{Reason: fmt.Sprintf("request body too large (limit %d bytes)", maxBytes)}
		}
		return nil, &MalformedError{Reason: "request body could not be read"}
	}
	return b, nil
}

// DecodeJSON reads and strictly decodes the request body into dst.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	b, err := ReadBody(w, r, maxBytes)
	if err != nil {
		return err
	}
	return DecodeJSONBytes(b, dst)
}

// DecodeJSONBytes strictly decodes one JSON object: unknown fields are rejected and
// numbers are kept as json.Number so money is never converted to float.
func DecodeJSONBytes(b []byte, dst any) error {
	if len(bytes.TrimSpace(b)) == 0 {
		return &MalformedError{Reason: "request body is empty"}
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	dec.UseNumber()

	if err := dec.Decode(dst); err != nil {
		return &MalformedError{Reason: describeDecodeError(err)}
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return &MalformedError{Reason: "request body must contain a single JSON object"}
	}
	return nil
}

func describeDecodeError(err error) string {
	var (
		syntaxErr *json.SyntaxError
		typeErr   *json.UnmarshalTypeError
	)
	switch {
	case errors.As(err, &syntaxErr):
		return fmt.Sprintf("invalid JSON at offset %d", syntaxErr.Offset)
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "invalid JSON: unexpected end of input"
	case errors.As(err, &typeErr):
		field := typeErr.Field
		if field == "" {
			field = "request body"
		}
		return fmt.Sprintf("field %s has the wrong type (expected %s)", field, typeErr.Type)
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		return "unknown field " + strings.TrimPrefix(err.Error(), "json: unknown field ")
	default:
		return "invalid JSON"
	}
}
