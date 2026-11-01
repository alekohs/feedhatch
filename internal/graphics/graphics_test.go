package graphics

import (
	"image"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi/kitty"
)

func TestPlaceholdersAreOneCellPerColumn(t *testing.T) {
	rows := Placeholders(42, 30, 4)
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4", len(rows))
	}
	for i, row := range rows {
		if w := lipgloss.Width(row); w != 30 {
			t.Fatalf("row %d is %d cells wide, want 30", i, w)
		}
		if !strings.HasPrefix(row, "\x1b[38;5;42m") || !IsPlaceholderRow(row) {
			t.Fatalf("row %d should carry the image ID in its color", i)
		}
		if !strings.Contains(row, string(kitty.Placeholder)+string(kitty.Diacritic(i))+string(kitty.Diacritic(0))) {
			t.Fatalf("row %d should start with the diacritics for row %d, column 0", i, i)
		}
	}
	if IsPlaceholderRow("plain text") {
		t.Fatal("plain text is not a placeholder row")
	}
}

func TestSequencesUseTmuxPassthroughOnlyInTmux(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))

	t.Setenv("TMUX", "")
	seq, err := Transmit(img, 20)
	if err != nil {
		t.Fatal(err)
	}
	// Transmit-only is the protocol's default action, so it carries no a= key and no placement.
	if !strings.HasPrefix(seq, "\x1b_G") || !strings.Contains(seq, "i=20") || strings.Contains(seq, "a=T") || strings.Contains(seq, "U=1") {
		t.Fatalf("transmit = %q", seq)
	}
	if place := Place(20, 10, 3); !strings.Contains(place, "a=p") || !strings.Contains(place, "U=1") || !strings.Contains(place, "c=10") || !strings.Contains(place, "r=3") {
		t.Fatalf("place = %q", place)
	}

	t.Setenv("TMUX", "/tmp/tmux-1000/default,1,0")
	seq, _ = Transmit(img, 20)
	if !strings.HasPrefix(seq, "\x1bPtmux;\x1b\x1b_G") || !strings.HasSuffix(seq, "\x1b\\") {
		t.Fatalf("inside tmux the sequence should be wrapped for passthrough: %q", seq)
	}
}

func TestSupportedOutsideTmux(t *testing.T) {
	cases := []struct {
		term, program, window string
		want                  bool
	}{
		{term: "xterm-kitty", want: true},
		{term: "xterm-ghostty", want: true},
		{term: "xterm-256color", program: "ghostty", want: true},
		{term: "xterm-256color", window: "1", want: true},
		{term: "xterm-256color", want: false},
		{term: "alacritty", want: false},
	}
	for _, c := range cases {
		t.Setenv("TMUX", "")
		t.Setenv("TERM", c.term)
		t.Setenv("TERM_PROGRAM", c.program)
		t.Setenv("KITTY_WINDOW_ID", c.window)
		if got := Supported(); got != c.want {
			t.Errorf("TERM=%s TERM_PROGRAM=%s KITTY_WINDOW_ID=%s: Supported() = %v, want %v", c.term, c.program, c.window, got, c.want)
		}
	}
}

func TestKittyTypeFromTmux(t *testing.T) {
	for termType, want := range map[string]bool{
		"ghostty 1.4.0-arch1": true,
		"kitty(0.38.1)":       true,
		"WezTerm 20240203":    false,
		"":                    false,
	} {
		if got := kittyType(termType); got != want {
			t.Errorf("kittyType(%q) = %v, want %v", termType, got, want)
		}
	}
}
