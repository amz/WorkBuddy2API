package main

import "testing"

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

// Shell metacharacters in a valid https URL are inert: exec.Command passes
// the URL as one argv element with no shell. This documents why the HIGH
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
// net/url.Parse lowercases the scheme, so HTTPS:// and Https:// parse to
// "https" and are accepted — a mixed-case login URL still opens the browser.
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
