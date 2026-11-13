package main

import (
	"reflect"
	"testing"
)

func TestCopySSELogged_NullChoicesPreserveTerminalMetadata(t *testing.T) {
	out := runCopySSELogged(t, `data: {"id":"upstream-id","model":"upstream-model","choices":null}`+"\n\n"+sseDone)
	chunks, done := parseSSELines(t, out)
	if done != 1 || len(chunks) < 2 {
		t.Fatalf("want metadata chunk, synthesized terminal and DONE: %s", out)
	}
	terminal := chunks[1]
	if terminal["id"] != "upstream-id" || terminal["model"] != "upstream-model" {
		t.Fatalf("synthesized terminal lost upstream identity: %v", terminal)
	}
	if got := terminalFinishReasons(chunks); got[0] != "length" {
		t.Fatalf("want terminal for choice 0: %v", got)
	}
}

func TestChoiceFinishReasons_DuplicateNullPreservesTerminal(t *testing.T) {
	for _, tt := range []struct {
		name string
		data string
		want map[int]string
	}{
		{"stop then null", `{"choices":[{"index":2,"finish_reason":"stop","finish_reason":null}]}`, map[int]string{2: "stop"}},
		{"tool calls then nulls", `{"choices":[{"index":2,"finish_reason":"tool_calls","finish_reason":null,"finish_reason":null}]}`, map[int]string{2: "tool_calls"}},
		{"null then stop", `{"choices":[{"index":2,"finish_reason":null,"finish_reason":"stop"}]}`, map[int]string{2: "stop"}},
		{"empty then null", `{"choices":[{"index":2,"finish_reason":"","finish_reason":null}]}`, map[int]string{}},
		{"null only", `{"choices":[{"index":2,"finish_reason":null}]}`, map[int]string{}},
		{"last string wins", `{"choices":[{"index":2,"finish_reason":"stop","finish_reason":"length","finish_reason":null}]}`, map[int]string{2: "length"}},
		{"empty replaces stop", `{"choices":[{"index":2,"finish_reason":"stop","finish_reason":"","finish_reason":null}]}`, map[int]string{}},
		{"mixed case", `{"choices":[{"index":2,"finish_reason":"stop","FINISH_REASON":null}]}`, map[int]string{2: "stop"}},
		{"independent choices", `{"choices":[{"index":2,"finish_reason":"stop","finish_reason":null},{"index":3,"finish_reason":null}]}`, map[int]string{2: "stop"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := choiceFinishReasons("data: " + tt.data); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCopySSELogged_DuplicateNullDoesNotSynthesizeLength(t *testing.T) {
	const line = `data: {"id":"terminal-id","model":"m","choices":[{"index":2,"delta":{},"finish_reason":"stop","finish_reason":null}]}`
	out := runCopySSELogged(t, line+"\n\n"+sseDone)
	chunks, done := parseSSELines(t, out)
	if done != 1 || len(chunks) != 1 {
		t.Fatalf("terminal followed by duplicate null must not synthesize a length chunk: %s", out)
	}
}

func TestCopySSELogged_CaseInsensitiveTerminalFields(t *testing.T) {
	const line = `data: {"Id":"case-id","MoDeL":"case-model","ChOiCeS":[{"InDeX":2,"FiNiSh_ReAsOn":"stop"}]}`
	out := runCopySSELogged(t, line+"\n\n"+sseDone)
	chunks, done := parseSSELines(t, out)
	if done != 1 || len(chunks) != 1 {
		t.Fatalf("recognized terminal must not produce synthetic length chunk: %s", out)
	}
	meta := chunkMetaFromLine(line, sseChunkMeta{})
	if meta.id != "case-id" || meta.model != "case-model" {
		t.Fatalf("capitalized fields lost metadata: %+v", meta)
	}
	if got := choiceFinishReasons(line); len(got) != 1 || got[2] != "stop" {
		t.Fatalf("want stop for choice 2: %v", got)
	}
}
