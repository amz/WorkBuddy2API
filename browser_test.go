package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

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

// Shell metacharacters in a valid https URL are inert: the opener receives the
// URL as one argv element with no shell. This documents why the HIGH
// command-injection finding is a false positive.
func TestIsSafeLoginURL_AcceptsShellMetacharactersInHTTPSURL(t *testing.T) {
	for _, raw := range []string{
		"https://x/?a=1&state=2; rm -rf /",
		"https://x/?a=`id`",
		"https://x/?a=$(whoami)",
	} {
		if !isSafeLoginURL(raw) {
			t.Errorf("rejected valid https URL with inert metacharacters %q", raw)
		}
	}
}

// Scheme matching must not depend on how the upstream spells the scheme.
// net/url.Parse lowercases the scheme, and isSafeLoginURL compares
// case-insensitively, so a mixed-case login URL still opens the browser.
func TestIsSafeLoginURL_AcceptsMixedCaseScheme(t *testing.T) {
	for _, raw := range []string{
		"HTTPS://www.workbuddy.ai/login?platform=workbuddy-ai&state=abc",
		"Https://www.workbuddy.ai/login",
		"HtTpS://x/y",
	} {
		if !isSafeLoginURL(raw) {
			t.Errorf("rejected valid login URL with mixed-case scheme %q", raw)
		}
	}
}

// captureOpenURL swaps the opener out for the duration of a test and returns
// a pointer to the argv slices it recorded.
func captureOpenURL(t *testing.T) *[][]string {
	t.Helper()
	var calls [][]string
	orig := openURL
	openURL = func(name string, args ...string) error {
		argv := append([]string{name}, args...)
		calls = append(calls, argv)
		return nil // pretend the first opener worked
	}
	t.Cleanup(func() { openURL = orig })
	return &calls
}

// The opener must receive the login URL as exactly one argv element of a
// constant executable. If startCmd ever routed through a shell, the URL would
// have to be split or passed to a shell binary — this test fails in that case,
// which the pure isSafeLoginURL assertions cannot detect.
func TestOpenBrowser_PassesURLAsSingleArgumentWithoutShell(t *testing.T) {
	calls := captureOpenURL(t)
	raw := "https://x/?a=1&state=2; rm -rf /"

	openBrowser(raw)

	if len(*calls) != 1 {
		t.Fatalf("openBrowser made %d opener calls, want 1: %v", len(*calls), *calls)
	}
	argv := (*calls)[0]
	switch argv[0] {
	case "rundll32", "open", "xdg-open":
	default:
		t.Errorf("opener executable = %q, want a constant opener, not a shell", argv[0])
	}
	for _, a := range argv {
		if a == "sh" || a == "bash" || a == "cmd" || a == "powershell" {
			t.Fatalf("opener argv contains a shell: %v", argv)
		}
	}
	last := argv[len(argv)-1]
	if last != raw {
		t.Errorf("URL passed to opener = %q, want the unmodified single argument %q", last, raw)
	}
	for _, a := range argv[1:] {
		if a == "; " || a == "rm" || a == "-rf" {
			t.Fatalf("URL was split into shell-like argv elements: %v", argv)
		}
	}
}

// An unsafe URL must not reach the opener at all.
func TestOpenBrowser_DoesNotExecOnUnsafeURL(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "-a Calculator", "javascript:alert(1)"} {
		calls := captureOpenURL(t)
		openBrowser(raw)
		if len(*calls) != 0 {
			t.Errorf("openBrowser(%q) invoked the opener %d times, want 0: %v", raw, len(*calls), *calls)
		}
	}
}

// startCmd must build a real exec.Cmd for the named binary with the URL as a
// single argv element and no shell. This inspects the actual *exec.Cmd (via
// the execCommand seam), so it fails if startCmd ever wraps the invocation in
// `sh -c` / `cmd /c` — which the pure isSafeLoginURL tests cannot detect.
func TestStartCmd_BuildsShellFreeCommand(t *testing.T) {
	var got *exec.Cmd
	orig := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		got = exec.Command(name, args...)
		return got
	}
	t.Cleanup(func() { execCommand = orig })

	// Avoid starting a real browser: swap the starter out for a no-op.
	origStart := execStart
	execStart = func(*exec.Cmd) error { return nil }
	t.Cleanup(func() { execStart = origStart })

	raw := "https://x/?a=1&state=2; rm -rf /"
	if err := startCmd("xdg-open", raw); err != nil {
		t.Fatalf("startCmd: %v", err)
	}
	if got == nil {
		t.Fatal("startCmd built no command")
	}

	if base := filepath.Base(got.Args[0]); base == "sh" || base == "bash" || base == "cmd" || base == "powershell" {
		t.Fatalf("startCmd routed through a shell: %q", got.Args)
	}
	if len(got.Args) != 2 || got.Args[1] != raw {
		t.Fatalf("argv = %q, want [xdg-open %q] (URL as one argument)", got.Args, raw)
	}
}
