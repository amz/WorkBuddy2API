package main

import (
	"context"
	"io"
	"net/http"
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
