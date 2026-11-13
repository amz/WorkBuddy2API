package main

import (
	"encoding/json"
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
		{"model wrong type", `{"model":42,"messages":[{"role":"user","content":"hi"}]}`, "request body must be valid JSON"},
		{"messages wrong type", `{"model":"m","messages":{}}`, "request body must be valid JSON"},
		{"messages string type", `{"model":"m","messages":"hi"}`, "request body must be valid JSON"},
		{"messages type before missing model", `{"messages":42}`, "request body must be valid JSON"},
		{"stream type before empty messages", `{"model":"m","messages":[],"stream":"true"}`, "request body must be valid JSON"},
		{"model empty", `{"model":"","messages":[{"role":"user","content":"hi"}]}`, "model is required"},
		{"model whitespace", `{"model":"   ","messages":[{"role":"user","content":"hi"}]}`, "model is required"},
		{"model missing", `{"messages":[{"role":"user","content":"hi"}]}`, "model is required"},
		{"model null", `{"model":null,"messages":[{"role":"user","content":"hi"}]}`, "model is required"},
		{"messages missing", `{"model":"m"}`, "messages must not be empty"},
		{"messages empty", `{"model":"m","messages":[]}`, "messages must not be empty"},
		{"messages null", `{"model":"m","messages":null}`, "messages must not be empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %q)", rec.Code, rec.Body.String())
			}
			var got struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("invalid error response: %v", err)
			}
			if got.Error.Message != tc.wantMsg {
				t.Fatalf("error message = %q, want exactly %q", got.Error.Message, tc.wantMsg)
			}
		})
	}
}

func TestHandleChat_NullStreamDefaultsToStreaming(t *testing.T) {
	up := &scriptedTransport{status: 200, body: "data: [DONE]\n\n"}
	am := fakeAuth("acct-null-stream", up)
	srv := NewServer(NewUpstreamClientPool(poolOf(am)), am, "", false, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(
		`{"model":"m","stream":null,"messages":[{"role":"user","content":"hi"}]}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("null stream should default to streaming: status=%d headers=%v body=%s",
			rec.Code, rec.Header(), rec.Body.String())
	}
}
