package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

// These catch whole-object re-marshalling, content substring replacement,
// and accidental rewriting outside direct choice finish_reason fields.
func TestNormalizeIntermediateFinishReason_ByteExact(t *testing.T) {
	for _, tt := range []struct{ name, in, want string }{
		{"spaced", `data: { "z":1, "choices" : [ { "finish_reason" : "" } ] }`, `data: { "z":1, "choices" : [ { "finish_reason" : null } ] }`},
		{"two", `data: {"choices":[{"finish_reason":""},{"index":1,"finish_reason":""}]}`, `data: {"choices":[{"finish_reason":null},{"index":1,"finish_reason":null}]}`},
		{"escaped content", `data: {"choices":[{"delta":{"content":"\"finish_reason\":\"\""},"finish_reason":""}]}`, `data: {"choices":[{"delta":{"content":"\"finish_reason\":\"\""},"finish_reason":null}]}`},
		{"root", `data: {"finish_reason":"","choices":[]}`, `data: {"finish_reason":"","choices":[]}`},
		{"nested", `data: {"choices":[{"delta":{"finish_reason":""}}]}`, `data: {"choices":[{"delta":{"finish_reason":""}}]}`},
		{"malformed", `data: {"choices":[{"finish_reason":""}],}`, `data: {"choices":[{"finish_reason":""}],}`},
		{"passthrough", ` data:  {"z": "<script>", "choices":[{"delta":{"content":"hello"}}]} `, ` data:  {"z": "<script>", "choices":[{"delta":{"content":"hello"}}]} `},
		{"duplicate choices", `data: {"choices":[{"finish_reason":""}],"choices":[{"finish_reason":"stop"}]}`, `data: {"choices":[{"finish_reason":""}],"choices":[{"finish_reason":"stop"}]}`},
		{"duplicate reason", `data: {"choices":[{"finish_reason":"","finish_reason":null}]}`, `data: {"choices":[{"finish_reason":"","finish_reason":null}]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeIntermediateFinishReason(tt.in); got != tt.want {
				t.Fatalf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestScanChunkData_JSONReference(t *testing.T) {
	// The reference deliberately uses encoding/json, not scanner helpers.
	for _, data := range []string{
		`{}`,
		`{"id":"chatcmpl-1","model":"m","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":""}]}`,
		`{"choices":[{"index":2,"finish_reason":"stop"},{"index":-1,"finish_reason":"length"},{}]}`,
		`{"id":"a\"b\\c\/\n\u0041\uD83D\uDE00","model":"\tmodel","choices":[{"finish_reason":null}]}`,
		`{"delta":{"id":"wrong","model":"wrong","choices":[{"index":8}]},"choices":[{"delta":{"finish_reason":"stop","index":99}}]}`,
		`{"choices":[{"index":0}],"choices":[{"index":3,"finish_reason":"stop"}]}`,
		`{"choices":[{"index":null,"finish_reason":null}]}`,
		`{"other":[true,false,null,-1.25e+2,{"key":"escaped\\\"text"}]}`,
		`{"id":42}`, `{"model":[]}`, `{"choices":{}}`,
		`{"choices":[{"index":"1"}]}`, `{"choices":[{"index":1.5}]}`,
		`{"choices":[{"index":1e0}]}`, `{"choices":[{"finish_reason":false}]}`,
		`{"choices":[3]}`, `{"choices":[{"finish_reason":""}],}`,
		`{"unknown":01}`, `{"unknown":trueX}`, `{"unknown":[1,]}`, `{"unknown":"unterminated}`,
		`[]`, ``, `{"choices":[`,
	} {
		t.Run(data, func(t *testing.T) {
			var ref struct {
				ID, Model string
				Choices   []struct {
					Index        int
					FinishReason string `json:"finish_reason"`
				}
			}
			err := json.Unmarshal([]byte(data), &ref)
			got, ok := scanChunkData(data)
			if ok != (err == nil) {
				t.Fatalf("ok=%v, JSON error=%v", ok, err)
			}
			if !ok {
				return
			}
			if got.id != ref.ID || got.model != ref.Model || len(got.choices) != len(ref.Choices) {
				t.Fatalf("scan=%+v, reference=%+v", got, ref)
			}
			for i, ch := range got.choices {
				if ch.index != ref.Choices[i].Index || ch.finish != ref.Choices[i].FinishReason {
					t.Fatalf("choice %d: scan=%+v, reference=%+v", i, ch, ref.Choices[i])
				}
				if ch.emptyFRSpan != [2]int{-1, -1} && data[ch.emptyFRSpan[0]:ch.emptyFRSpan[1]] != `""` {
					t.Fatalf("invalid empty finish span: %v", ch.emptyFRSpan)
				}
			}
		})
	}
}

func TestScanChunkData_AcceptedDivergences(t *testing.T) {
	for _, data := range []string{
		`{"id":"ok","choices":[]} trailing garbage`,
		"{\"id\":\"ok\",\"ignored\":\"raw\ncontrol\",\"choices\":[]}",
	} {
		got, ok := scanChunkData(data)
		if !ok || got.id != "ok" || len(got.choices) != 0 {
			t.Fatalf("scan(%q) = %+v, %v", data, got, ok)
		}
	}
	// Unlike encoding/json's struct decoder, the spec requires an array.
	if _, ok := scanChunkData(`{"choices":null}`); ok {
		t.Fatal("null choices accepted")
	}
	got, ok := scanChunkData(`{"choices":[{"finish_reason":""}]}`)
	if !ok || !reflect.DeepEqual(got.choices[0].emptyFRSpan, [2]int{29, 31}) {
		t.Fatalf("empty literal span: %+v, %v", got, ok)
	}
}

func BenchmarkScanChunkData(b *testing.B) {
	const data = `{"id":"chatcmpl-123","model":"claude","choices":[{"index":0,"delta":{"content":"some generated text"},"finish_reason":""}]}`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scanChunkData(data)
	}
}
