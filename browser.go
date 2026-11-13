package main

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
)

// openURL launches a URL opener. It is a package variable so tests can
// capture the exact argv instead of spawning a real browser.
var openURL = startCmd

// execCommand builds the *exec.Cmd to launch. It is a package variable so
// tests can inspect the real command (path + argv) without spawning a
// process, which is what proves no shell is involved.
var execCommand = exec.Command

// execStart starts the command. It is a package variable so tests can prevent
// a real process from launching.
var execStart = func(cmd *exec.Cmd) error { return cmd.Start() }

// startCmd launches a program detached, without a shell.
//
// NOTE: on Windows we must NOT route through `cmd /c start`, because the
// shell splits the login URL on `&` (e.g. `?platform=...&state=...` would
// lose the state param). exec.Command(...).Start() detaches fine on its own.
func startCmd(name string, args ...string) error {
	cmd := execCommand(name, args...)
	return execStart(cmd)
}

func hasStoredAuth(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	s := string(raw)
	return strings.Contains(s, "accessToken") && strings.Contains(s, "refreshToken")
}

func bufioWait() {
	_, _ = bufio.NewReader(os.Stdin).ReadByte()
}
