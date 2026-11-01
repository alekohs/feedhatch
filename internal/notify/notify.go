// Package notify shows desktop notifications through the operating system.
package notify

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

// run starts a command; tests replace it.
var run = func(name string, args ...string) error { return exec.Command(name, args...).Run() }

// Send shows a desktop notification: notify-send on Linux, osascript on macOS.
func Send(title, body string) error {
	switch runtime.GOOS {
	case "linux":
		return run("notify-send", "--app-name=feedhatch", title, body)
	case "darwin":
		return run("osascript", "-e", "display notification "+appleString(body)+" with title "+appleString(title))
	}
	return errors.ErrUnsupported
}

// appleString quotes s as an AppleScript string literal.
func appleString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
