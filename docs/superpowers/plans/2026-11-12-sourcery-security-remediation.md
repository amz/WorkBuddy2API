# Sourcery Security Findings Remediation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Resolve the 9 active Sourcery security findings on `amz/WorkBuddy2API` (4 groups): fix the 4 genuinely unpinned GitHub Actions, make the container's non-root intent explicit, add two low-risk defense-in-depth hardenings, and formally triage the false positives.

**Architecture:** This is a hardening/triage pass on an existing Go 1.23 stdlib-only binary, not a feature. Four independent investigations (one per Sourcery group) concluded: 5 of 9 findings are false positives (XSS ×3, command injection ×1, Dockerfile-root ×1), and 4 are real but low-effort (unpinned action refs). The code changes are deliberately minimal and split into three independent, separately testable units: CI action pinning (`.github/workflows/*.yml`), container declaration (`Dockerfile`), and Go defense-in-depth (`login.go`, `server.go`, `upstream.go` + tests). No API response bodies are changed — the proxy's OpenAI/Anthropic-compatible wire format is a hard constraint.

**Tech Stack:** Go 1.23 (`module WorkBuddy2API`, no `go.sum`, stdlib only), GitHub Actions, Docker multi-stage build (`golang:1.23-bookworm` → `gcr.io/distroless/static-debian12:nonroot`), `go test`/`go vet` (`package main`, table-driven, `httptest`), `gh` CLI, `docker`.

**Spec:** This plan is the spec. It argues from the four subagent investigation reports (summarised in "Findings & verdicts" below) and the Sourcery MCP scan of commit `afbf2a8725f80e3a4d854fbcae83dd904430c1e9` scoped to `amz/WorkBuddy2API`.

## Global Constraints

- Repo: `amz/WorkBuddy2API`; default branch `main`. Work on a new `ai/<slug>` branch; never push to `main` directly; open a PR/MR via push options or `gh pr create`.
- HEAD at time of writing: `5fcefe8`. The Sourcery scan commit `afbf2a8` is an ancestor. Line numbers in the findings refer to `afbf2a8`; **locate code by content, not by line number** (`release.yml:156` is now `:193`; `server.go:271/608` are now `:291/628`; `upstream.go:378` is now `:447`).
- Go version floor: `go 1.23` (`go.mod:3`). No new dependencies — stdlib only. Do not add a `go.sum`.
- CI (`.github/workflows/ci.yml`) runs `go vet ./...`, `go test -count=1 ./...`, `go build -o WorkBuddy2API.exe .` on `windows-latest`. Tests must be cross-platform-safe (no process spawning, no Unix-only paths, no shell).
- **No native Go toolchain on this devbox** (`go` is not on PATH), but Docker 29.1.3 with `golang:1.23-bookworm` cached locally is available. Run all Go verification in a container (exact command in "Environment" below), or let CI do it.
- **Never alter response bodies of `handleChat` / `copySSELogged` / `copySSE`.** No `html.EscapeString`, no `html/template`, no re-encoding of upstream error bytes. These are JSON/SSE API responses, not HTML; escaping corrupts the OpenAI/Anthropic wire format.
- Do not forward upstream `Content-Type` in `handleChat`'s error path — the hard-coded `application/json` is what prevents HTML rendering.
- Do not add `RUN useradd` / `RUN chown` to the Dockerfile's distroless final stage (no shell, no useradd, no package manager — the build would fail).
- Do not trigger the `Docker` or `Release` workflows for validation; they push real images/releases. `release.yml` triggers on every push to `main` (only `*.md` is ignored), so workflow edits must go through a PR.
- Use `owner/repo@<40-hex-sha> # vX.Y.Z` format for action pins; SHAs must be re-resolved from the tag at implementation time (the values below are a 2026-11-12 snapshot).
- Sourcery MCP tools available for triage: `bulk_update_security_issues` / `bulk_update_security_issue_groups` (server: `sourcery`, owner `amz`). Triage is a state change — get explicit user approval before executing it (it is listed as an optional final task).

## Findings & verdicts (from the 4 investigations)

| Group | Sev | Type | Verdict | Action |
|---|---|---|---|---|
| 158586045 — XSS via `ResponseWriter.Write` (3 issues: 162878425 `server.go:291`, 162878426 `server.go:628`, 162878427 `upstream.go:447`) | HIGH | SAST | **False positive.** All three sinks set `Content-Type: application/json` or `text/event-stream` *before* writing; there are zero HTML responses in the repo (no `text/html`, no `html/template`, no `html.EscapeString` anywhere). `copySSE`/`copyBody` are dead code (no callers). Escaping would corrupt JSON/SSE. | Triage IGNORED; optional `nosniff` middleware; optionally delete dead helpers. |
| 158586044 — command injection (`browser.go:16`, id 162878424) | HIGH | SAST | **False positive.** `exec.Command(name, args...)` uses no shell; `name` is always one of three constants (`rundll32`/`open`/`xdg-open`) from `login.go:203-212`; the "untrusted" value is an `authUrl` from a TLS-pinned upstream, passed as a single argv element. Residual LOW risk: unsafe URL scheme handed to OS openers. | Triage IGNORED; optional pure `isSafeLoginURL` helper + tests. |
| 158586039 — root user in Dockerfile (id 162878419, `Dockerfile:1-29`) | MEDIUM | IAC | **False positive / already mitigated.** Final stage is `gcr.io/distroless/static-debian12:nonroot` (UID 65532); `CKV_DOCKER_3` is a syntactic check that only looks for a literal `USER` token. Sourcery's suggested `RUN useradd` would break the build. | Add explicit `USER 65532:65532` (one line, declarative no-op); then triage. |
| 158586040 — unpinned third-party actions (4 issues: 162878420 `docker.yml:50`, 162878421 `:52`, 162878422 `:57`, 162878423 `release.yml:193`) | MEDIUM | SAST | **REAL.** All 4 are mutable major tags (`docker/*@v3`, `@v6`, `softprops/action-gh-release@v2`). `release.yml` has `contents: write`; `docker.yml` holds `DOCKERHUB_USERNAME`/`DOCKERHUB_TOKEN` and pushes `:latest`. | Pin all 13 `uses:` refs to SHAs; add Dependabot config. |

## Environment (verification commands)

```bash
# Go verification in a container (no native go on this devbox):
docker run --rm -v "$PWD":/src -w /src golang:1.23-bookworm \
  sh -c 'go vet ./... && go test -count=1 ./... && go build ./...'

# Workflow / SHA verification (gh is authenticated as devbox-netlife-pl[bot]):
gh api repos/OWNER/REPO/git/ref/tags/TAG --jq '.object.type, .object.sha'
gh api repos/OWNER/REPO/commits/TAG --jq .sha
gh workflow list --all
```

`actionlint`, `checkov` are not installed locally — use the container or CI for those.

## Review Focus

1. **A workflow-only push to `main` triggers a real release.** `release.yml` has `paths-ignore: ["*.md"]` only — pin changes MUST land via a PR, never a direct push to `main`. Test: Task 1's verification step asserts the diff contains only `.github/workflows/*` + `.github/dependabot.yml` and that no workflow was triggered.
2. **A retagged/pinned SHA that does not match the intended version silently downgrades or upgrades an action.** Test: Task 1 Step 4 re-resolves each SHA from its tag and compares; Task 1 Step 5 asserts every `uses:` ref is 40-hex with a `# vX.Y.Z` comment.
3. **An "XSS fix" that escapes upstream bytes breaks OpenAI/Anthropic clients.** Test: Task 4's `TestHandleChat_UpstreamErrorBodyIsNotHTMLEscaped` and `TestCopySSELogged_ContentTypeAndRawPayloadPreserved` assert byte-for-byte passthrough of a `<script>` payload.
4. **A `RUN` added to the distroless stage fails the build.** Test: Task 3 Step 4 asserts the final stage contains no `RUN`, and `docker build` succeeds.
5. **URL validation that rejects the real `authUrl` breaks interactive login.** Test: Task 2's `TestIsSafeLoginURL_AcceptsHTTPSWithQuery` and `TestIsSafeLoginURL_AcceptsWorkBuddyLoginURL`; host is deliberately NOT pinned because OAuth login URLs may live on another host.

---

### Task 1: Pin all GitHub Actions to commit SHAs

**Files:**
- Modify: `.github/workflows/docker.yml:29,50,52,57`
- Modify: `.github/workflows/release.yml:43,48,66,77,127,133,193`
- Modify: `.github/workflows/ci.yml:13,16`
- Create: `.github/dependabot.yml`

**Interfaces:**
- Consumes: nothing (independent).
- Produces: no code interface — the deliverable is that every `uses:` in `.github/workflows/` is `owner/repo@<40-hex-sha> # vX.Y.Z`, and a weekly `github-actions` Dependabot config exists so pins do not rot.

- [ ] **Step 1: Create the branch**

```bash
git checkout -b ai/pin-actions-and-security-hardening
```

- [ ] **Step 2: Re-resolve every tag to its current 40-char SHA**

For each of `actions/checkout@v4`, `actions/setup-go@v5`, `actions/upload-artifact@v4`, `actions/download-artifact@v4`, `docker/setup-buildx-action@v3`, `docker/login-action@v3`, `docker/build-push-action@v6`, `softprops/action-gh-release@v2`, run:

```bash
gh api repos/OWNER/REPO/git/ref/tags/TAG --jq '.object.type, .object.sha'
gh api repos/OWNER/REPO/commits/TAG --jq .sha
```

Record the SHA from `commits/TAG --jq .sha`. For an annotated tag (`object.type == "tag"`), dereference with `gh api repos/OWNER/REPO/git/tags/<sha> --jq .object.sha`. Confirm the human version by matching the SHA against `gh api "repos/OWNER/REPO/tags?per_page=100" --jq '[.[] | select(.commit.sha=="SHA") | .name] | join(", ")'`. **Do not copy the snapshot values blindly** — 2026-11-12 values were: `checkout` v4.4.0 `11d5960a326750d5838078e36cf38b85af677262`, `setup-go` v5.6.0 `40f1582b2485089dde7abd97c1529aa768e1baff`, `upload-artifact` v4.6.2 `ea165f8d65b6e75b540449e92b4886f43607fa02`, `download-artifact` v4.3.0 `d3f86a106a0bac45b974a628896c90dbdf5c8093`, `setup-buildx-action` v3.12.0 `8d2750c68a42422c14e847fe6c8ac0403b4cbd6f`, `login-action` v3.7.0 `c94ce9fb468520275223c153574b00df6fe4bcc9`, `build-push-action` v6.19.2 `10e90e3645eae34f1e60eeb005ba3a3d33f178e8`, `action-gh-release` v2.6.2 `3bb12739c298aeb8a4eeaf626c5b8d85266b0e65`. Pin the tag's commit, NOT the repo's default-branch HEAD.

- [ ] **Step 3: Replace all 13 `uses:` refs**

Keep every existing `with:`/`env:`/`name:` unchanged. Pin all 13 (the 4 flagged third-party ones AND the 9 `actions/*` ones) so a future re-scan does not re-flag the same class. Format each as:

```yaml
      - uses: docker/login-action@<40-hex-sha>  # v3.7.0
```

Match by the `uses:` text, not line number. Note `release.yml:193` is `softprops/action-gh-release@v2` (the finding says `:156` — stale); `release.yml:156` is now a shell line and must not be touched.

- [ ] **Step 4: Verify each SHA maps to the version in its comment**

For each pinned ref, run `gh api repos/OWNER/REPO/git/ref/tags/TAG --jq .object.sha` and assert it equals the SHA in the file. Expected: 8/8 equal. Any mismatch means a wrong or stale pin — fix it.

- [ ] **Step 5: Verify no unpinned refs remain**

```bash
grep -rn 'uses:' .github/workflows/ | grep -vE '@[0-9a-f]{40}'   # expected: no output
```

Also assert every pinned line has a `# vX.Y.Z` comment (Dependabot uses it to display and update the human version).

- [ ] **Step 6: Create `.github/dependabot.yml`**

```yaml
version: 2
updates:
  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "weekly"
```

Without this, SHA pins never receive security updates. Dependabot's `github-actions` ecosystem updates SHA pins and preserves the `# vX.Y.Z` comment.

- [ ] **Step 7: Verify workflow validity without triggering a run**

Do NOT call `gh workflow run` (Docker pushes `:latest`; Release publishes binaries). Instead:

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.23-bookworm \
  sh -c 'go install github.com/rhysd/actionlint/cmd/actionlint@latest && "$(go env GOPATH)/bin/actionlint" .github/workflows/*.yml'
gh workflow list --all
gh workflow view Docker
gh workflow view Release
```

Expected: actionlint reports no errors; `gh workflow view` shows both workflows without a parse error. If actionlint cannot be installed, fall back to a YAML parse: `ruby -e 'require "yaml"; Dir[".github/workflows/*.yml"].each { |f| YAML.load_file(f); puts "OK #{f}" }'`.

- [ ] **Step 8: Confirm the diff is workflow-only and commit**

```bash
git diff --stat     # expected: only .github/workflows/*.yml and .github/dependabot.yml
git add .github/workflows/ .github/dependabot.yml
git commit -m "ci: pin all GitHub Actions to commit SHAs and enable Dependabot"
```

---

### Task 2: Validate the login URL before handing it to OS openers

**Files:**
- Modify: `login.go` (add `net/url` import; add pure helper; call it at the top of `openBrowser`, currently `login.go:203`)
- Test: Create `browser_test.go`

**Interfaces:**
- Consumes: nothing (independent of other tasks).
- Produces: `func isSafeLoginURL(raw string) bool` in `login.go` — pure, portable, no process spawning. `openBrowser` returns early (printing the existing "copy it manually" message) when the helper returns false.

- [ ] **Step 1: Write the failing test** in `browser_test.go` (`package main`, table-driven, stdlib only):

```go
func TestIsSafeLoginURL_AcceptsHTTPSWithQuery(t *testing.T) {
	if !isSafeLoginURL("https://www.workbuddy.ai/login?platform=workbuddy-ai&state=abc") {
		t.Fatal("valid https login URL with & in query was rejected")
	}
}

func TestIsSafeLoginURL_RejectsUnsafeSchemes(t *testing.T) {
	for _, raw := range []string{
		"file:///etc/passwd", "javascript:alert(1)", "smb://host/share",
		"/tmp/evil.desktop", `C:\Windows\evil.exe`,
	} {
		if isSafeLoginURL(raw) {
			t.Errorf("accepted unsafe URL %q", raw)
		}
	}
}

func TestIsSafeLoginURL_RejectsOptionInjection(t *testing.T) {
	for _, raw := range []string{"-a Calculator", "-R /", "--args foo"} {
		if isSafeLoginURL(raw) {
			t.Errorf("accepted macOS open option %q", raw)
		}
	}
}

func TestIsSafeLoginURL_RejectsMalformed(t *testing.T) {
	for _, raw := range []string{"", "https://", "://x", "not a url"} {
		if isSafeLoginURL(raw) {
			t.Errorf("accepted malformed URL %q", raw)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.23-bookworm \
  sh -c 'go test -count=1 -run TestIsSafeLoginURL ./...'
```

Expected: FAIL — `undefined: isSafeLoginURL`.

- [ ] **Step 3: Implement `isSafeLoginURL(raw string) bool` in `login.go`**

Add `net/url` to the imports. The helper must return true only when `url.Parse` succeeds, `u.Scheme == "https"`, and `u.Host != ""`. Do **not** pin the host (OAuth/SSO login may live on a different host); do **not** allow `http`. This single check eliminates `file://`/`javascript:`/local `.desktop` handler abuse and macOS `open` leading-`-` option confusion. Call it at the top of `openBrowser`:

```go
func openBrowser(rawURL string) {
	if !isSafeLoginURL(rawURL) {
		fmt.Fprintln(os.Stderr, "(refusing to open unsafe login URL; copy it manually)")
		return
	}
	// ... existing literal command table unchanged ...
}
```

Keep `startCmd` (`browser.go:15-18`) and the three constant openers unchanged — there is no command injection there and no shell is involved.

- [ ] **Step 4: Run the tests to verify they pass**

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.23-bookworm \
  sh -c 'go test -count=1 ./...'
```

Expected: PASS (all existing tests plus the new ones). Note the existing tests are cross-platform; this helper is pure, so CI on `windows-latest` is safe.

- [ ] **Step 5: Commit**

```bash
git add login.go browser_test.go
git commit -m "security: validate login URL scheme before launching OS opener"
```

---

### Task 3: Declare the non-root container user explicitly

**Files:**
- Modify: `Dockerfile` (final/runtime stage only)

**Interfaces:**
- Consumes: nothing.
- Produces: no code interface — the deliverable is `docker inspect --format '{{.Config.User}}'` returns `65532:65532` and `CKV_DOCKER_3` passes.

- [ ] **Step 1: Add the `USER` directive**

Insert exactly one line in the **final stage**, after `EXPOSE 61021` (`Dockerfile:25`) and immediately before `ENTRYPOINT` (`Dockerfile:29`):

```dockerfile
# gcr.io/distroless/static-debian12:nonroot already defaults to uid/gid 65532.
# Declare it explicitly for scanners (CKV_DOCKER_3) and k8s runAsNonRoot admission.
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/workbuddy2api"]
```

Use numeric UID/GID, not `USER nonroot` (kubelet cannot always resolve named users). Do **not** put it in the build stage (`golang:1.23-bookworm`, line 6) — only `/out/workbuddy2api` is copied across the `FROM` boundary. Do **not** add `RUN useradd`/`RUN chown` (distroless has no shell/useradd; the build would fail) and do **not** change the `:nonroot` base tag.

- [ ] **Step 2: Assert the final stage has no `RUN` and exactly one `USER`**

```bash
grep -nE '^\s*(FROM|USER|RUN|ENTRYPOINT)\b' Dockerfile
```

Expected: two `FROM`, one `USER 65532:65532` after the second `FROM`, zero `RUN` in the final stage, one `ENTRYPOINT`.

- [ ] **Step 3: Build and inspect the image config**

```bash
docker build -t wb2api:security-test .
docker inspect --format '{{.Config.User}}' wb2api:security-test
```

Expected: `65532:65532`. (A `docker run ... id` check will NOT work — `static:nonroot` has no shell or `id` binary; use `docker inspect`.)

- [ ] **Step 4: Verify the scanner rule passes**

```bash
docker run --rm -v "$PWD":/src -w /src python:3-slim \
  sh -c 'pip install -q checkov && checkov -f Dockerfile --check CKV_DOCKER_3'
```

Expected: PASSED for `CKV_DOCKER_3`. This is the actual acceptance criterion — the rule checks only for the literal `USER` token.

- [ ] **Step 5: Confirm the change is declarative (no new layer, no size change)**

```bash
docker history wb2api:security-test --no-trunc | head
```

Expected: no new filesystem layer from `USER` (config metadata only). The effective UID is unchanged from the `:nonroot` base, so volume ownership at `/auth` is unaffected.

- [ ] **Step 6: Commit**

```bash
git add Dockerfile
git commit -m "build: declare non-root container user explicitly (CKV_DOCKER_3)"
```

---

### Task 4: Defense-in-depth `nosniff` header, dead-code cleanup, and no-escaping regression tests

**Files:**
- Modify: `server.go` (add `noSniff` middleware near `Handler()`, chain it outermost)
- Modify: `upstream.go` (delete dead `copySSE` and `copyBody`, currently `upstream.go:438-462`)
- Test: Modify `server_test.go` (new handler-level cases), `sse_stream_test.go` (recorder-returning helper + no-escaping case)

**Interfaces:**
- Consumes: nothing (independent).
- Produces: `func noSniff(next http.Handler) http.Handler` in `server.go`, applied as the outermost wrapper in `Handler()`.

- [ ] **Step 1: Write the failing tests**

In `server_test.go` add:

```go
func TestHandler_NoSniffOnAllRoutes(t *testing.T) {
	srv := &Server{} // no upstream needed: these paths respond before any fetch
	for _, path := range []string{"/healthz", "/v1/models", "/v1/chat/completions", "/"} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options = %q, want nosniff", path, got)
		}
	}
}

func TestHandleChat_UpstreamErrorBodyIsNotHTMLEscaped(t *testing.T) {
	raw := `<html><script>alert(1)</script></html>`
	up := &scriptedTransport{status: 400, body: raw} // pool_failover_test.go:30
	am := fakeAuth("acct-a", up)                      // pool_failover_test.go:46
	p := poolOf(am)                                   // pool_failover_test.go:73
	forceStart(p, 0)                                 // pool_failover_test.go:82
	srv := NewServer(NewUpstreamClientPool(p), am, "", false, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if got := rec.Body.String(); got != raw {
		t.Fatalf("upstream error body was altered: %q", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}
```

Note to implementer: `poolOf`/`forceStart`/`fakeAuth` are defined in `pool_failover_test.go` and the construction above mirrors `TestHandleChat_RecordsVerdictUnderServingAccount` (`pool_failover_test.go:371`). The assertion (byte-for-byte `raw` passthrough) is the load-bearing part.

In `sse_stream_test.go`, add a `runCopySSELoggedRec(t, upstream) *httptest.ResponseRecorder` variant of the existing `runCopySSELogged` helper (`sse_stream_test.go:25`) that returns the recorder instead of just the body, then add `TestCopySSELogged_ContentTypeAndRawPayloadPreserved`: feed a `data:` line whose `delta.content` is `<script>alert(1)</script>`, assert `Content-Type == "text/event-stream"`, `parseSSELines` succeeds, and the decoded content still equals the raw `<script>` string. Reuse `parseSSELines` (`sse_stream_test.go:42`) and the `sseIntermediate`/`sseTerminal`/`sseDone` constants (`sse_stream_test.go:19-23`).

That unit test calls `copySSELogged` directly, which bypasses `Handler()` and its middleware, so it cannot observe `X-Content-Type-Options` — the `noSniff` wrapper sets that, not `copySSELogged`. Do **not** assert `nosniff` there. Instead add `TestHandleChat_StreamSetsNosniffHeader`, which drives a real `stream:true` request through `srv.Handler()` (same `fakeAuth`/`poolOf`/`forceStart` construction as above, with `t.Setenv(probeEnv, "0")`) and asserts status 200, `Content-Type: text/event-stream`, `X-Content-Type-Options: nosniff`, one `[DONE]`, and the decoded `<script>` payload intact.

- [ ] **Step 2: Run the tests to verify they fail**

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.23-bookworm \
  sh -c 'go test -count=1 -run "TestHandler_NoSniffOnAllRoutes|TestHandleChat_UpstreamErrorBodyIsNotHTMLEscaped|TestCopySSELogged_ContentTypeAndRawPayloadPreserved" ./...'
```

Expected: FAIL — `nosniff` header absent (the two body-preservation tests should already pass; they are regression guards, not red tests).

- [ ] **Step 3: Implement `noSniff(next http.Handler) http.Handler` in `server.go`**

```go
func noSniff(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}
```

Chain it as the outermost wrapper in `Handler()` (`server.go:28-44`): the function currently builds `h` via `var h http.Handler = s.requireAPIKey(mux)`, optionally wraps it with `s.limiter.middleware(h)`, then `return h`. Change that final line to `return noSniff(h)` so the header is set on every route including the auth-denied and rate-limited responses. This is a header-only change: no body bytes touched, no SSE framing impact, no client-visible shape change. Do **not** add escaping anywhere. Do **not** replace `handleChat`'s raw error passthrough with `writeError` (`server.go:750`) — that changes the API error shape and is a breaking change.

- [ ] **Step 4: Delete the dead helpers in `upstream.go`**

Remove `func copySSE(...)` and `func copyBody(...)` (`upstream.go:438-462`). Verified: `grep -rn 'copySSE\|copyBody' --include=*.go .` shows only their definitions and `copySSELogged` call sites (a different function). They have zero callers. This removes the third flagged sink entirely with zero runtime effect. Keep `copySSELogged` (`server.go:600`) untouched.

- [ ] **Step 5: Run the full verification suite**

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.23-bookworm \
  sh -c 'go vet ./... && go test -count=1 ./... && go build ./...'
```

Expected: all PASS. In particular the existing `TestCopySSELogged_*`, `TestHandleChat_RecordsVerdictUnderServingAccount`, and `TestHandleModels_UsesPooledFetchWithFailover` must still pass — they detect any accidental body-shape change. Optionally `go test -count=1 -race ./...` (SSE paths use goroutines/flushers).

- [ ] **Step 6: Commit**

```bash
git add server.go upstream.go server_test.go sse_stream_test.go
git commit -m "security: add nosniff header, remove dead SSE helpers, guard response passthrough"
```

---

### Task 5: Full-branch verification and PR

**Files:** none (verification + submission only).

**Interfaces:**
- Consumes: the four commits from Tasks 1–4.

- [ ] **Step 1: Run the full CI-equivalent suite in a container**

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.23-bookworm \
  sh -c 'go vet ./... && go test -count=1 ./... && go build -o /tmp/WorkBuddy2API.exe .'
```

Expected: all PASS.

- [ ] **Step 2: Confirm no unpinned actions and no `RUN` in the final Docker stage**

```bash
grep -rn 'uses:' .github/workflows/ | grep -vE '@[0-9a-f]{40}'   # expected: no output
grep -nE '^\s*(FROM|USER|RUN|ENTRYPOINT)\b' Dockerfile
```

- [ ] **Step 3: Open the PR**

```bash
git push -u origin ai/pin-actions-and-security-hardening \
  -o merge_request.create \
  -o merge_request.target=main \
  -o merge_request.title="Resolve Sourcery security findings" \
  -o merge_request.remove_source_branch
```

On GitHub use the service-account token (the repo is under the `amz` installation, id `181059562`):

```bash
export GITHUB_APP_ORG=amz GITHUB_APP_INSTALLATION_ID=181059562
export GH_TOKEN=$(/home/dev/.local/bin/github-app-token)
git push "https://x-access-token:${GH_TOKEN}@github.com/amz/WorkBuddy2API.git" ai/pin-actions-and-security-hardening
gh pr create --repo amz/WorkBuddy2API --head ai/pin-actions-and-security-hardening --base main \
  --title "Resolve Sourcery security findings" --body-file <(printf '%s\n' 'Pins all GitHub Actions to SHAs, declares the non-root container user, hardens login URL handling, adds nosniff, and removes dead SSE helpers. XSS and command-injection findings are false positives (non-HTML API proxy responses; constant exec binary).')
```

Do not push directly to `main` — `release.yml` triggers on every push to `main` and would publish a release.

- [ ] **Step 4: Re-run the Sourcery scan / verify finding status after merge**

Expected: group 158586040 becomes SOLVED; group 158586039 becomes SOLVED once `USER` is present. Groups 158586045 and 158586044 will remain ACTIVE as scanner false positives — proceed to Task 6 only with explicit user approval.

---

### Task 6 (OPTIONAL — requires explicit user approval): Triage false positives in Sourcery

This is a state change on the Sourcery server, not a code change. **Ask the user before executing.**

**Files:** none. Uses the `sourcery` MCP tools via `execute` (`owner: "amz"`).

- [ ] **Step 1: Confirm with the user** that groups 158586045 (XSS, 3 issues) and 158586044 (command injection, 1 issue) should be marked IGNORED, and group 158586039 (Dockerfile root) should be left for the scanner to auto-solve.

- [ ] **Step 2: Bulk-update the 4 false-positive issues**

```
tools.sourcery.bulk_update_security_issues({
  owner: "amz",
  ids: [162878425, 162878426, 162878427, 162878424],
  status: "IGNORED",
  reason: "False positive: non-HTML API proxy (application/json, text/event-stream); XSS rule fires on raw upstream passthrough. Command injection: exec.Command uses constant binary names, no shell, upstream TLS-pinned URL as single argv element. URL scheme validation added as defense-in-depth."
})
```

Expected: `updated_ids` contains all 4, `failed` empty.

- [ ] **Step 3: Verify**

```
tools.sourcery.get_security_issue_stats({ owner: "amz", repository: "amz/WorkBuddy2API" })
```

Expected: `ignored_count >= 4`, `active_count` reduced. Report the exact before/after counts to the user.

---

## Self-review notes

- **Spec coverage:** All 9 issues / 4 groups are addressed — real ones fixed (Tasks 1, 3), false positives hardened where cheap and cheaply triaged (Tasks 2, 4, 6). No finding is silently dropped.
- **Step scan:** Each step is one checkable action. Test steps give exact assertions; code steps give exact signatures/file placements; verification steps give exact commands and expected output.
- **Type consistency:** Only one new Go signature (`isSafeLoginURL(raw string) bool`, `noSniff(next http.Handler) http.Handler`); both are used exactly as defined. No cross-task name mismatch.
- **Proportion:** The plan is longer than the finding list because it encodes the four investigations' evidence; code blocks are confined to the two small Go helpers, the Dockerfile line, the Dependabot YAML, and test skeletons — not transcripts of the program.
- **Execution order:** Tasks 1–4 are independent and could be parallelised, but they all touch the same branch, so sequential commits are simpler. Task 5 gates them; Task 6 is opt-in.
