package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// capturingTransport records the request body sent upstream and replies with
// a scripted response.
type capturingTransport struct {
	status int
	body   string
	got    string
}

func TestHandleAnthropic_ReasoningTokenFloor(t *testing.T) {
	product := filepath.Join(t.TempDir(), "product.json")
	if err := os.WriteFile(product, []byte(`{"models":[{"id":"test-reasoning","credits":"x1","onlyReasoning":true}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(productJSONEnv, product)
	for _, tt := range []struct {
		name   string
		budget int
		want   float64
	}{
		{"raise small budget", 16, 4096},
		{"preserve larger budget", 8192, 8192},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tr := &capturingTransport{status: 200, body: "data: [DONE]\n\n"}
			am := fakeAuth("reasoning", tr)
			srv := NewServer(NewUpstreamClient(am), am, "", false, nil)
			body, err := json.Marshal(map[string]any{
				"model": "test-reasoning", "max_tokens": tt.budget, "stream": false,
				"messages": []any{map[string]any{"role": "user", "content": "hi"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(body)))
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			var sent map[string]any
			if err := json.Unmarshal([]byte(tr.got), &sent); err != nil {
				t.Fatalf("upstream body: %v", err)
			}
			if sent["max_tokens"] != tt.want {
				t.Fatalf("upstream max_tokens=%v, want %v", sent["max_tokens"], tt.want)
			}
		})
	}
}

func (c *capturingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	c.got = string(b)
	return &http.Response{
		StatusCode: c.status,
		Body:       io.NopCloser(strings.NewReader(c.body)),
		Header:     make(http.Header),
		Request:    r,
	}, nil
}

// TestChatWithOptions_ProbeKeepsStreamFalse pins the availability-probe path:
// an explicit stream:false survives rewriting (probe must not stream), and the
// payload is marshalled exactly once — the body sent upstream must carry the
// probe's fields verbatim.
func TestChatWithOptions_ProbeKeepsStreamFalse(t *testing.T) {
	tr := &capturingTransport{status: 200, body: "data: [DONE]\n\n"}
	am := fakeAuth("probe", tr)
	c := NewUpstreamClient(am)

	payload := map[string]any{
		"model":    "m",
		"stream":   false,
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	}
	resp, _, err := c.chatWithOptions(context.Background(), payload, chatOptions{skipReasoningFloor: true})
	if err != nil {
		t.Fatalf("chatWithOptions: %v", err)
	}
	defer resp.Body.Close()

	if !strings.Contains(tr.got, `"stream":false`) {
		t.Fatalf("upstream body lost stream:false: %s", tr.got)
	}
	if !strings.Contains(tr.got, `"model":"m"`) {
		t.Fatalf("upstream body lost model: %s", tr.got)
	}
}
