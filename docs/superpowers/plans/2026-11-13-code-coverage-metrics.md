# Code Coverage Metrics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Go test code coverage a real, visible number on every CI run and pull request, with no new third-party services or credentials.

**Architecture:** Coverage is measured by Go's built-in `-coverprofile` in the existing `ci.yml` Test step, summarized by `go tool cover -func`, surfaced in the workflow job summary (`$GITHUB_STEP_SUMMARY`) and logs, and archived as `coverage.out` plus an HTML report via `actions/upload-artifact`. A coverage badge is added to the README. This is a CI/reporting-only change: no product code, no dependencies, no response bodies, no API changes. Measurement is visibility-only by default (no pass/fail threshold).

**Tech Stack:** Go 1.23 (`module WorkBuddy2API`, no `go.sum`, stdlib only), GitHub Actions, `go test -coverprofile` / `go tool cover` (stdlib toolchain), `actions/upload-artifact` (SHA-pinned), `GITHUB_STEP_SUMMARY`, `windows-latest` runner, Docker `golang:1.23-bookworm` for local verification (no native Go on the devbox).

**Spec:** This plan is the spec. It argues from the repo's current state: `.github/workflows/ci.yml` runs `go vet ./...`, `go test -count=1 ./...`, `go build -o WorkBuddy2API.exe .`; there is no coverage config, no Codecov/Coveralls, no `Makefile`, and no coverage badge. Design decisions (approach A, visibility-only, badge URL correction) were settled in conversation on 2026-11-13.

## Confirmed decisions (2026-11-13)

1. **Surface = A: job summary + artifacts + badge.** No Codecov/Coveralls, no `codecov.yml`, no external service, no third-party GitHub App or token, no PR bot comment. The coverage number lives in `$GITHUB_STEP_SUMMARY` (rendered on the CI run page linked from the PR), the step logs, and the downloadable `coverage-report` artifact. Approach B (Codecov) and C (`GITHUB_TOKEN` PR comment) are **rejected** and must not be implemented.
2. **Gate = visibility-only.** No coverage threshold, no `fail` on a low percentage, no baseline enforcement. CI must never fail because of a coverage number. The measured `total:` percentage is reported as an informal baseline for the maintainer's information only.

Both match the defaults the tasks below were written against, so no task changes are needed.

## Global Constraints

- Repo: `amz/WorkBuddy2API`; default branch `main`. Work on a new `ai/<slug>` branch; never push to `main` directly; open a PR with `gh pr create`.
- Current branch at time of writing: `ai/pin-actions-and-security-hardening` (HEAD `f5ab419`). This plan is a **separate, independent** change — start a new branch from `main` (or from the merged result of that branch), not on top of it, so the coverage PR stands alone.
- Go version floor: `go 1.23` (`go.mod:3`). No new dependencies — stdlib only. Do not add a `go.sum`.
- CI (`.github/workflows/ci.yml`) runs on `windows-latest`. All shell steps are PowerShell by default on that runner; write any shell snippets to be valid PowerShell, or set `shell: bash` explicitly. Tests must remain cross-platform-safe (no process spawning, no Unix-only paths, no shell).
- **No native Go toolchain on this devbox** (`go` is not on PATH). Run all Go verification in a container:
  `docker run --rm -v "$PWD":/src -w /src golang:1.23-bookworm sh -c 'go vet ./... && go test -count=1 -coverprofile=coverage.out -covermode=atomic ./... && go tool cover -func=coverage.out'`
- **Confirmed decisions (2026-11-13):** Surface = **A** (job summary + artifacts + static badge, no external service); Gate = **visibility-only** (no threshold, CI never fails on a coverage number). B (Codecov) and C (`GITHUB_TOKEN` PR comment) are rejected.
- **Base = standalone from `main`** (explicit human-partner choice). Consequence: `main`'s `ci.yml` has unpinned `actions/checkout@v4` / `actions/setup-go@v5`; Task 3 re-pins them so the "every `uses:` is 40-hex" assertion holds. This overlaps `ai/pin-actions-and-security-hardening`; a merge conflict on `ci.yml` is possible and resolvable.
- **Windows `$GITHUB_STEP_SUMMARY` path guard:** on `windows-latest` with `shell: bash`, the summary path is Windows-style and a raw `>>` redirect fails. Task 2's step converts it with `cygpath -u` (guarded by `command -v`).
- **Action pinning rule (from the security-remediation work):** every `uses:` must be `owner/repo@<40-hex-sha> # vX.Y.Z`. Re-resolve SHAs from tags at implementation time (values in this plan were re-resolved 2026-11-13: `upload-artifact` v4 → `ea165f8d65b6e75b540449e92b4886f43607fa02` / v4.6.2; `checkout` v4 → `11d5960a326750d5838078e36cf38b85af677262` / v4.4.0; `setup-go` v5 → `40f1582b2485089dde7abd97c1529aa768e1baff` / v5.6.0).
- `release.yml` triggers on every push to `main` (`paths-ignore: ["*.md"]` only). All workflow/README edits in this plan MUST land via a PR, never a direct push to `main`. Do not trigger the `Docker` or `Release` workflows for validation.
- **Badge URL correction:** README badges currently target `ErfanBagheri404/WorkBuddy2API` (upstream), but the working remote is `amz/WorkBuddy2API`. The new coverage badge MUST use `amz/WorkBuddy2API`. Do not silently "fix" the other three badges in this change — that is out of scope; note it as a follow-up.
- Do not alter any Go product code, response bodies of `handleChat` / SSE helpers, the Dockerfile, or `release.yml` / `docker.yml`.
- `coverage.out`, `coverage.html`, and any generated report files must be gitignored (build artifacts, not source).

## File Structure

| File | Responsibility |
|---|---|
| `.github/workflows/ci.yml` (modify, Test + reporting steps) | Produce `coverage.out`, compute the total, write `$GITHUB_STEP_SUMMARY`, upload artifacts. Single source of truth for measurement. |
| `.gitignore` (modify) | Exclude generated coverage artifacts from commits. |
| `README.md` (modify, badge block lines 7–10) | Add the coverage badge pointing at `amz/WorkBuddy2API`. |
| `codecov.yml` — **not created** (approach A) | Deliberately absent; no external service. |

No new Go files. No `Makefile`. No `go.sum`. No new workflow file.

## Review Focus

The five failure modes most likely to bite a maintainer, each pinned by a verification step:

1. **A coverage step that fails on Windows breaks CI for every PR.** The existing runner is `windows-latest`; `go tool cover -func` and file redirection behave differently in PowerShell. Test: Task 2 verification runs the exact step logic in the `windows-latest` CI job (and locally via the Linux container), asserting the step exits 0 and prints a parseable `total:` line.
2. **`-covermode=atomic` vs `set` on the concurrent SSE paths.** The codebase streams with goroutines/flushers; `set` mode can miscount or panic under `-race`/concurrency. Test: Task 1 uses `-covermode=atomic` and the verification asserts the profile's `mode:` line is `atomic`.
3. **An unpinned `actions/upload-artifact` reintroduces the finding the last PR fixed.** Test: Task 3 verification asserts every `uses:` ref in `ci.yml` is 40-hex with a `# vX.Y.Z` comment, and re-resolves the SHA from the tag.
4. **Generated coverage artifacts get committed.** Test: Task 4 verification runs `git status --short` after a local coverage run and asserts `coverage.out` / `coverage.html` do not appear.
5. **A badge pointed at the wrong repo shows stale/blank data.** Test: Task 5 verification greps README for `amz/WorkBuddy2API` in the new badge and asserts no new `ErfanBagheri404` coverage link.

---

### Task 1: Measure coverage in the Test step

**Files:**
- Modify: `.github/workflows/ci.yml:23-24` (the `Test` step)
- Test: none (workflow change; verified by CI + local container run)

**Interfaces:**
- Consumes: the existing `go test -count=1 ./...` invocation.
- Produces: `coverage.out` at the repo root, in Go coverprofile format with `mode: atomic`, consumed by Task 2.

- [ ] **Step 1: Add coverage flags to the Test step**

Change the `Test` step's `run:` from `go test -count=1 ./...` to:

```yaml
      - name: Test
        run: go test -count=1 -covermode=atomic -coverprofile=coverage.out ./...
```

Rationale: `-count=1` keeps the existing no-cache behavior; `-covermode=atomic` is correct for the concurrent SSE/streaming code; `-coverprofile` writes the profile the later steps consume. No `shell:` needed — the command is pure `go`, identical in PowerShell and bash.

- [ ] **Step 2: Verify locally in the Go container**

Run:
```bash
docker run --rm -v "$PWD":/src -w /src golang:1.23-bookworm \
  sh -c 'go test -count=1 -covermode=atomic -coverprofile=coverage.out ./... && head -1 coverage.out && go tool cover -func=coverage.out | tail -1'
```
Expected: all existing tests PASS; the first profile line is `mode: atomic`; the last line is `total: (statements) XX.X%`. Record the percentage as the informal baseline (informational only — no gate).

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: measure test coverage with go test -coverprofile"
```

---

### Task 2: Report the number in the job summary

**Files:**
- Modify: `.github/workflows/ci.yml` (add a step immediately after the `Test` step)

**Interfaces:**
- Consumes: `coverage.out` from Task 1.
- Produces: a `total:` line in the CI logs and a Markdown coverage table in `$GITHUB_STEP_SUMMARY`, visible on the PR's CI check.

- [ ] **Step 1: Add the summary step**

Insert after the `Test` step:

```yaml
      - name: Coverage summary
        shell: bash
        run: |
          total=$(go tool cover -func=coverage.out | tail -1)
          echo "Coverage: $total"
          summary="$GITHUB_STEP_SUMMARY"
          if command -v cygpath >/dev/null 2>&1; then
            summary=$(cygpath -u "$summary")
          fi
          {
            echo "## Code coverage"
            echo ""
            echo '```'
            go tool cover -func=coverage.out
            echo '```'
            echo ""
            echo "**$total**"
          } >> "$summary"
```

Rationale: `shell: bash` is set explicitly so the redirection and `$(...)` are portable and identical to local verification, avoiding PowerShell parsing differences (Review Focus #1). `go tool cover -func` prints a per-function table plus a final `total:` line — that full table is what makes the metric useful, not just the headline number. `$GITHUB_STEP_SUMMARY` renders on the workflow run page that the PR links to.

**Windows path guard (added during implementation):** on `windows-latest` with `shell: bash`, `$GITHUB_STEP_SUMMARY` is a Windows-style path (`D:\a\_temp\...`); a raw `>> "$GITHUB_STEP_SUMMARY"` fails with "No such file or directory" because bash eats the backslashes. `cygpath -u` (shipped with Git for Windows, guarded by `command -v` so Linux is unaffected) converts it to a usable Unix path. This is the difference between the summary actually appearing on the PR and a red CI step.

- [ ] **Step 2: Verify the reporting logic**

Run locally by extracting the **exact `run:` block** from `ci.yml` and executing it in the container (do not retype it — bash interprets backticks differently inside double quotes, which silently produces an empty table):

```bash
awk '/^      - name: Coverage summary/{found=1} found && /^        run: \|/{inrun=1; next} inrun && /^      - name:/{exit} inrun {sub(/^          /, ""); print}' \
  .github/workflows/ci.yml > /tmp/coverage-step.sh
docker run --rm -v "$PWD":/src -w /src -v /tmp/coverage-step.sh:/work/step.sh:ro \
  -e GITHUB_STEP_SUMMARY=/work/summary.md golang:1.23-bookworm \
  bash -c 'mkdir -p /work && go test -count=1 -covermode=atomic -coverprofile=coverage.out ./... && bash /work/step.sh && \
    grep -q "^## Code coverage" /work/summary.md && \
    grep -q "WorkBuddy2API" /work/summary.md && \
    grep -q "total:" /work/summary.md && tail -3 /work/summary.md'
```
Expected: `Coverage: total: (statements) XX.X%` printed; the summary contains the heading, per-function rows (e.g. `NewAccountPool 86.2%`), and the `total:` line. Then confirm on the PR's CI run that the "Coverage summary" step is green and the run page shows the "Code coverage" table. This is what answers "is it visible on the PR" — the acceptance check for the whole plan.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: publish coverage table to the job summary"
```

---

### Task 3: Archive coverage artifacts

**Files:**
- Modify: `.github/workflows/ci.yml` (add a step after the `Coverage summary` step, before/after `Build`)

**Interfaces:**
- Consumes: `coverage.out` from Task 1.
- Produces: a downloadable `coverage-report` artifact containing `coverage.out` and `coverage.html`, pinned to the pinned `actions/upload-artifact` SHA.

- [ ] **Step 1: Add a dedicated HTML-report step and resolve the action SHA**

Insert this step immediately before the upload step (self-contained: it does not modify Task 2's step):

```yaml
      - name: Generate HTML coverage report
        run: go tool cover -html=coverage.out -o coverage.html
```

Then resolve the `actions/upload-artifact` pin at implementation time:

```bash
gh api repos/actions/upload-artifact/git/ref/tags/v4 --jq '.object.type, .object.sha'
# if type == "tag", dereference:
gh api repos/actions/upload-artifact/git/tags/<sha> --jq .object.sha
```

Use the resulting 40-hex SHA with a `# v4.x.y` comment. Do not copy a value from the older security-remediation plan doc without re-resolving.

- [ ] **Step 2: Add the upload step**

```yaml
      - name: Upload coverage report
        uses: actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02 # v4.6.2
        with:
          name: coverage-report
          path: |
            coverage.out
            coverage.html
          if-no-files-found: error
```

Rationale: `if-no-files-found: error` makes a broken profile a visible failure rather than a silently empty artifact. The HTML report lets a reviewer browse uncovered lines without a local Go toolchain. The SHA was re-resolved on 2026-11-13 (`gh api repos/actions/upload-artifact/git/ref/tags/v4` → `ea165f8d65b6e75b540449e92b4886f43607fa02`, matched to tags `v4.6.2, v4`).

- [ ] **Step 3: Re-pin the two pre-existing mutable refs (standalone-from-`main` consequence)**

Because this branch is cut from `main` (not the security branch), `ci.yml` still has `actions/checkout@v4` and `actions/setup-go@v5`. Task 3's verification asserts every `uses:` is 40-hex, so re-pin both here, using SHAs re-resolved from tags on 2026-11-13:

```yaml
      - uses: actions/checkout@11d5960a326750d5838078e36cf38b85af677262 # v4.4.0

      - name: Set up Go
        uses: actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff # v5.6.0
```

These are identical to the pins on `ai/pin-actions-and-security-hardening`; a later merge of that PR may conflict on `ci.yml`, which is resolvable.

- [ ] **Step 4: Verify pinning and artifacts**

Run: `grep -nE 'uses:' .github/workflows/ci.yml` — assert every ref is 40-hex with a `# vX.Y.Z` comment (Review Focus #3). On the PR's CI run, confirm the `coverage-report` artifact is downloadable and contains both files.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: upload coverage.out and HTML report as build artifact"
```

---

### Task 4: Ignore generated coverage files

**Files:**
- Modify: `.gitignore`

**Interfaces:**
- Consumes: the filenames `coverage.out`, `coverage.html` from Tasks 1 and 3.
- Produces: a clean `git status` after local coverage runs.

- [ ] **Step 1: Read `.gitignore` and append the entries**

`.gitignore` currently has a `# Go` section at line 20 containing only `/vendor/`. Append the two generated artifacts under that section, after `/vendor/`:

```
# Go coverage artifacts (generated by CI)
coverage.out
coverage.html
```

- [ ] **Step 2: Verify the ignore works**

Run:
```bash
docker run --rm -v "$PWD":/src -w /src golang:1.23-bookworm \
  sh -c 'go test -count=1 -covermode=atomic -coverprofile=coverage.out ./... && go tool cover -html=coverage.out -o coverage.html'
git status --short
```
Expected: `coverage.out` and `coverage.html` do NOT appear as untracked (Review Focus #4). Only `.gitignore` shows as modified.

- [ ] **Step 3: Commit**

```bash
git add .gitignore
git commit -m "chore: ignore generated Go coverage artifacts"
```

---

### Task 5: Add the coverage badge to the README

**Files:**
- Modify: `README.md:7-10` (the badge block inside `<div align="center">`)

**Interfaces:**
- Consumes: nothing from earlier tasks (static badge).
- Produces: a coverage badge visible on the repo landing page and in PR diffs of the README.

- [ ] **Step 1: Choose the badge source**

With approach A there is no Codecov/Coveralls data source, so the badge cannot be a live percentage. Use a static "coverage measured in CI" badge that links to the CI workflow, e.g.:

```markdown
[![Coverage](https://img.shields.io/badge/coverage-measured%20in%20CI-blue?logo=go)](https://github.com/amz/WorkBuddy2API/actions/workflows/ci.yml)
```

Rationale: honest (it states the number is in CI, not a fake live percentage) and points at `amz/WorkBuddy2API`, the actual working remote (Review Focus #5). If the human partner later chooses approach B (Codecov), swap this for the Codecov badge `https://codecov.io/gh/amz/WorkBuddy2API/branch/main/graph/badge.svg`.

- [ ] **Step 2: Add the badge line**

Insert it into the badge block after the CI badge (line 7), before the Release badge. Leave the other three badges untouched — correcting their `ErfanBagheri404` URLs is out of scope; note it as a follow-up in the PR description.

- [ ] **Step 3: Verify the URL**

Run: `grep -n 'coverage\|ErfanBagheri404' README.md`
Expected: exactly one new coverage badge line, containing `amz/WorkBuddy2API` and no `ErfanBagheri404` on that line; the badge block (lines 7–11) has 5 badges and the 2 upstream-URL badges (CI, Release) are byte-identical to `main`. Note: `grep -c 'ErfanBagheri404' README.md` returns **4**, not 3 — 2 badges plus 2 body links (release download line 35, clone line 199); do not assert on a repo-wide count.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: add coverage badge linking to CI"
```

---

### Task 6: Open the PR and confirm visibility (acceptance)

**Files:**
- None (git/GitHub operations)

**Interfaces:**
- Consumes: the five commits above on the `ai/<slug>` branch.
- Produces: a PR whose CI run displays the coverage summary.

- [ ] **Step 1: Push and open the PR**

```bash
git push -u origin ai/<slug>
gh pr create --repo amz/WorkBuddy2API --head ai/<slug> --base main \
  --title "ci: report Go code coverage on every PR" \
  --body "Adds -coverprofile to CI, publishes a per-function coverage table to the job summary, archives coverage.out + HTML as artifacts, and adds a coverage badge. Visibility only (no threshold). Workflow/README edits land via PR to avoid triggering the release workflow."
```
Note: GitHub App installation for `amz` is `181059562`; ensure the token is minted for that org (`export GITHUB_APP_ORG=amz` / `GITHUB_APP_INSTALLATION_ID=181059562`) before `gh pr create`, per the devbox GitHub access notes.

- [ ] **Step 2: Acceptance check — answer "is it visible on the PR?"**

On the PR: open the **CI** check → the run page shows a **"Code coverage"** section with the per-function table and a `total: ...%` line; the `coverage-report` artifact is downloadable; the `Coverage summary` step logs `Coverage: total: ...%`. Report the measured percentage back to the maintainer as the informal baseline. If any of these are missing, the change is not done.

- [ ] **Step 3: Confirm no unintended workflows fired**

Assert no `Docker`/`Release` workflow runs were triggered by the branch push, and that the diff contains only `.github/workflows/ci.yml`, `.gitignore`, and `README.md`.

---

## What this plan deliberately does NOT do

- No Codecov/Coveralls account, token, `codecov.yml`, or PR bot comment (approach A).
- No coverage threshold / failing gate; no baseline enforcement (visibility-only).
- No diff/"new code" coverage, no trend graphs, no per-PR delta.
- No changes to Go product code, tests, Dockerfile, `release.yml`, `docker.yml`, or the three existing upstream-URL badges.
- No `Makefile`, no scripts, no new dependencies, no `go.sum`.

## Self-Review

1. **Spec coverage:** Every stated deliverable (measure, summarize, archive, ignore, badge, PR visibility) maps to Tasks 1–6; the two decisions are confirmed in "Confirmed decisions" and the rejected alternatives are explicitly excluded.
2. **Step scan:** Each step is one checkable action with an exact command, expected output, or exact YAML. No "TBD"/"handle edge cases" lines.
3. **Type consistency:** `coverage.out` / `coverage.html` / `coverage-report` names are consistent across Tasks 1–4; `-covermode=atomic` is consistent between the CI step and all local verification commands.
4. **Review Focus:** All five items have a named verification step (Windows/CI green, atomic mode, SHA pin, gitignore, badge URL).
5. **Proportion:** The plan is CI/reporting glue; code blocks are short YAML/shell snippets, not a transcript of a program. Longer than the code it changes, but that code is ~15 lines of YAML — appropriate for a plan that also carries constraints and verification.
