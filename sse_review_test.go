package main

import (
	"encoding/json"
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

// The pre-optimization choiceFinishReasons used *string, not string. JSON null
// clears a pointer even after an earlier duplicate terminal reason.
func TestChoiceFinishReasons_DuplicateNullMatchesPointerDecoder(t *testing.T) {
	for _, data := range []string{
		`{"choices":[{"index":2,"finish_reason":"stop","finish_reason":null}]}`,
		`{"choices":[{"index":2,"finish_reason":null,"finish_reason":"stop"}]}`,
		`{"choices":[{"index":2,"finish_reason":"","finish_reason":null}]}`,
	} {
		var old struct {
			Choices []struct {
				Index        int     `json:"index"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &old); err != nil {
			t.Fatal(err)
		}
		want := make(map[int]string)
		for _, ch := range old.Choices {
			if ch.FinishReason != nil && *ch.FinishReason != "" {
				want[ch.Index] = *ch.FinishReason
			}
		}
		if got := choiceFinishReasons("data: " + data); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got %v, old pointer decoder %v", data, got, want)
		}
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
