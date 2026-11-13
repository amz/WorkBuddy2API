package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleChat_Validation pins the 400 validation contract of
// /v1/chat/completions: exact messages, in the current order. The scripted
// upstream returns 200 [DONE], so any 200 here means a bad request leaked
// through to upstream.
func TestHandleChat_Validation(t *testing.T) {
	up := &scriptedTransport{status: 200, body: "data: [DONE]\n\n"}
	am := fakeAuth("acct-a", up)
	srv := NewServer(NewUpstreamClientPool(poolOf(am)), am, "", false, nil)

	cases := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{"stream string type", `{"model":"m","stream":"true","messages":[{"role":"user","content":"hi"}]}`, "request body must be valid JSON"},
		{"model wrong type", `{"model":42,"messages":[{"role":"user","content":"hi"}]}`, "model is required"},
		{"model empty", `{"model":"","messages":[{"role":"user","content":"hi"}]}`, "model is required"},
		{"model whitespace", `{"model":"   ","messages":[{"role":"user","content":"hi"}]}`, "model is required"},
		{"model missing", `{"messages":[{"role":"user","content":"hi"}]}`, "model is required"},
		{"messages missing", `{"model":"m"}`, "messages must not be empty"},
		{"messages empty", `{"model":"m","messages":[]}`, "messages must not be empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %q)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.wantMsg) {
				t.Fatalf("body %q does not contain %q", rec.Body.String(), tc.wantMsg)
			}
		})
	}
}
