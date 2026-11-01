package notify

import (
	"runtime"
	"slices"
	"testing"
)

func TestSendRunsTheSystemsNotifier(t *testing.T) {
	var got []string
	previous := run
	run = func(name string, args ...string) error {
		got = append([]string{name}, args...)
		return nil
	}
	t.Cleanup(func() { run = previous })

	if err := Send(`The "Go" Blog`, "2 nya artiklar"); err != nil {
		t.Skipf("no notifier on %s: %v", runtime.GOOS, err)
	}
	switch runtime.GOOS {
	case "linux":
		if want := []string{"notify-send", "--app-name=feedhatch", `The "Go" Blog`, "2 nya artiklar"}; !slices.Equal(got, want) {
			t.Fatalf("ran %q, want %q", got, want)
		}
	case "darwin":
		if want := `display notification "2 nya artiklar" with title "The \"Go\" Blog"`; got[0] != "osascript" || got[2] != want {
			t.Fatalf("ran %q", got)
		}
	}
}

func TestAppleString(t *testing.T) {
	if got := appleString(`a "b" \c`); got != `"a \"b\" \\c"` {
		t.Fatalf("got %s", got)
	}
}
