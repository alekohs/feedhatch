package main

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestLoggerShowsComponentAsPrefix(t *testing.T) {
	t.Setenv("JOURNAL_STREAM", "")
	var out strings.Builder
	logger := newLogger(&out, time.TimeOnly).With("component", "server", "socket", "x.sock")

	logger.Info("fetched feed", "feed", "Arch Linux", "new", 3)

	line := strings.TrimSpace(out.String())
	want := regexp.MustCompile(`^\d\d:\d\d:\d\d INFO +<feedhatch/main_test\.go:\d+> server: fetched feed socket=x\.sock feed="Arch Linux" new=3$`)
	if !want.MatchString(line) {
		t.Fatalf("line = %q", line)
	}
}

func TestLoggerDropsTimeUnderSystemd(t *testing.T) {
	t.Setenv("JOURNAL_STREAM", "8:12345")
	var out strings.Builder

	newLogger(&out, time.TimeOnly).Warn("fetch failed")

	if line := strings.TrimSpace(out.String()); !regexp.MustCompile(`^WARN +<`).MatchString(line) {
		t.Fatalf("line = %q, want no timestamp when the journal adds its own", line)
	}
}
