package tui

import "testing"

func TestParseInterval(t *testing.T) {
	valid := map[string]int64{
		"15": 15, "45m": 45, "90 min": 90, "2h": 120, "2 t": 120, "1h30m": 90, "1h 30m": 90,
		"1d": 1440, "1d12h": 2160, "1w": 10080, "2v": 20160, " 3H ": 180,
	}
	for in, want := range valid {
		if got, ok := parseInterval(in); !ok || got != want {
			t.Errorf("parseInterval(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "0", "0m", "abc", "2x", "h2", "-5", "1.5h", "100w"} {
		if got, ok := parseInterval(in); ok {
			t.Errorf("parseInterval(%q) = %d, want an error", in, got)
		}
	}
}

func TestFormatIntervalRoundTrips(t *testing.T) {
	cases := map[int64]string{15: "15m", 60: "1h", 90: "1h30m", 1440: "1d", 2160: "1d12h", 10080: "1w", 20160: "2w", 1441: "1d1m"}
	for minutes, want := range cases {
		if got := formatInterval(minutes); got != want {
			t.Errorf("formatInterval(%d) = %q, want %q", minutes, got, want)
		}
		if back, ok := parseInterval(want); !ok || back != minutes {
			t.Errorf("%q should read back as %d, got %d", want, minutes, back)
		}
	}
}

func TestIntervalFormAcceptsUnits(t *testing.T) {
	m := feedModel(t)
	m = press(m, "i")
	if m.form.values[3] != "15m" {
		t.Fatalf("the form should show the current interval as %q, got %q", "15m", m.form.values[3])
	}
	m = press(m, "ctrl+u")
	m = paste(m, "2h")
	m = press(m, "enter")
	if m.form != nil || m.feeds[m.feedIndex].interval != 120 {
		t.Fatalf("2h should be saved as 120 minutes, got %d", m.feeds[m.feedIndex].interval)
	}
	if m = press(m, "i"); m.form.values[3] != "2h" {
		t.Fatalf("editing again should show %q, got %q", "2h", m.form.values[3])
	}
}
