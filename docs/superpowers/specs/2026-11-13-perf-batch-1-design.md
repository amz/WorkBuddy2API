# Perf Batch 1 — Design (#1–#5)

Date: 2026-11-13
Scope: approved fixes #1, #2, #3, #4, #5 only. Deferred: #8 (per-model `reasoning_effort:"none"` opt-in), #9, #10, #11.

Baseline: first byte ~1.15 s through the proxy. Target gain ~70–100 ms/request (~6–8%).

## #2 — Single-pass request body handling

Problem: a chat request body is encoded/decoded 4 times (probe struct decode, re-marshal, upstream decode, re-marshal).

Change:
- `handleChat` does ONE `json.Unmarshal` into `payload map[string]any` (replaces both the probe struct and the second unmarshal).
- Validation from the map, in the current order, preserving exact 400 messages:
  - `model` present, string, non-empty after TrimSpace → else `400 "model is required"`.
  - `messages` present, `[]any`, non-empty → else `400 "messages must not be empty"`.
  - `stream`, if present, must be JSON bool → else `400 "request body must be valid JSON"` (preserves the current `"stream":"true"` 400; map unmarshal alone would not reject type errors, hence the explicit check).
- `clientWantsStream(payload)`: `stream` absent → true; else the bool value.
- `payload["stream"] = true` (upstream only supports streaming).
- Desensitize unchanged.
- Call `Chat(r.Context(), payload)`.
- `Chat` / `chatWithOptions` take `map[string]any`. `chatWithOptions` drops its own Unmarshal; keeps: stream default (`stream`, if present and bool-false, stays false — availability probe path), system-prepend, `reasoningTokenFloor(payload)` (already takes a map, catalog.go:304), single `json.Marshal` at the end.
- Callers updated:
  - `handleAnthropic` (server.go:812): drop `openaiBody, _ := json.Marshal(payload)` (used only at 819) and pass `payload` directly.
  - `probeModel` (availability.go:239–247): pass map directly, drop the Marshal.
  - `pool_failover_test.go:342,359`: pass `map[string]any` literals.

## #3 — Zero-cost logging when disabled

Problem: logging work (summariseRequest, mutex, fmt) runs even when disabled.

Change:
- `Logger` gains `on atomic.Bool`.
- `EnableLogging` single-exit: stores `f != nil` into `on`, then the existing body.
- New `LoggingEnabled() bool` = atomic load.
- `logLine`: check `LoggingEnabled()` first (return if false), then mutex; nil-check the writer under the lock.
- `handleChat`: gate the whole `summariseRequest` + logLine block under `if LoggingEnabled()`.
- Existing `logging_test.go` (redaction, LoggingEnabled usage at 99/105) must keep passing.

## #5 — Connection pool sizing

Problem: default `MaxIdleConnsPerHost` (2) causes TLS re-handshakes on bursty sequential requests (~30 ms each).

Change: in `newHTTPClient` transport config: `MaxIdleConnsPerHost: 32`, `MaxIdleConns: 100`. The streaming client inherits via `newStreamingHTTPClient`. Pinned with a transport-values test in `stream_client_test.go`.

## #4 — DNS fallback correctness

Problem: `knownUpstreamIPs` is a stale hardcode (`43.163.17.187`); live `www.workbuddy.ai` resolves to `43.169.32.91`. `failingResolver` fallback always returns that stale IP, and `DialContext` always takes `ips[0]` despite its "random" comment.

Change:
- Re-verify with `dig +short www.workbuddy.ai` at implementation time; update `knownUpstreamIPs` to the live-resolved set.
- Extract `pickIP(ips []string) string`: empty → `""`; `math/rand.Intn(len(ips))`; `strings.TrimSuffix(ip, ".")`. Used by both the fallback branch and the resolved branch of `DialContext` (makes the randomize comment true).
- Tests in `stream_client_test.go`: `pickIP` returns members of the input across many draws (and empty→""), plus a `failingResolver` fallback test showing the fallback yields an IP from `knownUpstreamIPs`. Check existing coverage first to avoid duplicates.

## #1 — Byte-scan SSE hot path

Problem: every SSE data line goes through up to 5–7 `json.Unmarshal` calls; measured 43.2 ms vs 1.3 ms per 2000 chunks for the scan.

Change: new file `sse_scan.go`:

```go
type sseChoiceScan struct {
    index        int    // 0 when absent (mirrors encoding/json)
    finish       string // finish_reason as-is (absent/null → "")
    emptyFRSpan  [2]int // byte span of the "" literal when finish_reason is exactly ""; {-1,-1} otherwise
}
type sseChunkScan struct {
    id, model string
    choices   []sseChoiceScan
}
func scanChunkData(data string) (sseChunkScan, bool)
```

Scanner rules (single pass over the bytes):
- Root must be `{`; trailing garbage after the root object → accepted (divergence, below).
- `id`, `model`: tracked only at root; wrong JSON type → ok=false (matches old unmarshal error). Needs full string decoding (escapes incl. `\uXXXX` + surrogate pairs).
- `choices`: must be an array; not-array → ok=false. `index`/`finish_reason` tracked only for direct elements. Duplicate `choices` member resets the slice (last-wins, matches decode semantics). Non-integer `index` (e.g. string) → ok=false (old code failed the whole line too). `finish_reason` null → treated absent (no span; old code also made no change). Wrong-typed tracked values → ok=false.
- Strings being skipped (e.g. content) are scanned to the unescaped closing quote without escape validation (raw control chars accepted — divergence, below).

Helpers keep their signatures; each goes: `sseData` → existing bail conditions → `scanChunkData`; `ok=false` → old nil/meta-unchanged/line-unchanged behavior:
- `normalizeIntermediateFinishReason` (server.go:486–534): keep the `strings.Contains(data, "\"finish_reason\"")` pre-check; collect `emptyFRSpan`s; none → line unchanged; else surgically replace each `""` span with `null` and return `"data: " + fixed`. This drops the current whole-line re-serialize side effect (which sorted map keys) — byte-identical otherwise, including the required `""`→`null` fix.
- `chunkMetaFromLine`, `choiceFinishReasons`, and the `writeTerminalChunks`/`pendingChoices` inputs use the scan instead of per-line `json.Unmarshal`.

`finishReasonFromLine` (logging.go) stays JSON-based.

Accepted divergences (documented; old code behaved differently, all benign for OpenAI/SSE payloads):
1. Trailing garbage after the root JSON object is accepted (old: whole line failed).
2. Raw control characters inside skipped strings accepted (old: failed).
3. Whitespace layout is fully supported (test exists: spaced `"finish_reason" : ""`).

## Testing strategy

- `sse_stream_test.go` (15 tests via `parseSSELines`, incl. escaped-`finish_reason`-inside-content, arbitrary whitespace, raw `<script>` content) is the primary safety net for #1 and must pass unchanged.
- New `sse_scan_test.go`: byte-exact normalize cases (byte-identical else, `""` with spaces → `null`, two choices, root-level non-choice empty string untouched, escaped content untouched, malformed → line unchanged) and scan-vs-local-JSON-reference agreement on a corpus table.
- New `validation_test.go`: `"stream":"true"` → 400, `"model":42` → 400, model empty → 400.
- `pool_failover_test.go`, `logging_test.go`, `auth.go` tests keep passing.
- One `-race` run over the suite for #3's atomic.

## Benchmarks

Harness: `/home/dev/work/<session-id>/probe/bench.py` (first_any/first_reason/first_text/total/chunks/finish) against `http://127.0.0.1:61021/v1` (0.1.14 container baseline) vs locally built binary. Expected: ~70–100 ms improvement on first-token latency.
