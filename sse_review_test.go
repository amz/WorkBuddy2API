package main

import (
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
