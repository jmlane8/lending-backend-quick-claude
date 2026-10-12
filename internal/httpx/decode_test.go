package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type decodeTarget struct {
	Name   string      `json:"name"`
	Amount json.Number `json:"amount"`
	Nested struct {
		Count int `json:"count"`
	} `json:"nested"`
}

func TestDecodeJSON(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		max        int64
		wantErr    bool
		wantReason string
	}{
		{"valid", `{"name":"a","amount":1.50,"nested":{"count":2}}`, 1024, false, ""},
		{"unknown field", `{"name":"a","foo":1}`, 1024, true, `unknown field "foo"`},
		{"wrong type", `{"name":123}`, 1024, true, "name"},
		{"wrong nested type", `{"nested":{"count":"x"}}`, 1024, true, "nested.count"},
		{"invalid json", `{"name":`, 1024, true, "invalid JSON"},
		{"empty body", ``, 1024, true, "empty"},
		{"trailing data", `{"name":"a"} {"name":"b"}`, 1024, true, "single JSON object"},
		{"too large", `{"name":"` + strings.Repeat("x", 100) + `"}`, 32, true, "too large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			var dst decodeTarget
			err := DecodeJSON(w, r, &dst, tt.max)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if dst.Amount.String() != "1.50" {
					t.Errorf("Amount = %q, want exact text 1.50 (UseNumber)", dst.Amount)
				}
				return
			}
			var me *MalformedError
			if !errors.As(err, &me) {
				t.Fatalf("err = %v (%T), want *MalformedError", err, err)
			}
			if !strings.Contains(me.Reason, tt.wantReason) {
				t.Errorf("reason %q does not contain %q", me.Reason, tt.wantReason)
			}
		})
	}
}
