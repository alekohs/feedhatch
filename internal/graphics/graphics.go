// Package graphics shows images in terminals that speak the kitty graphics protocol
// (kitty, Ghostty), through its Unicode placeholders: an image is sent once, and the
// terminal draws it wherever placeholder characters with its ID appear in the text.
package graphics

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// IDs are 256-color indexes, since the placeholders' foreground color carries the
// image ID and 256 colors survive tmux; 0–15 are the terminal's own palette.
const (
	FirstID = 16
	LastID  = 255
)

// Supported reports whether this terminal can show images. Inside tmux the outer
// terminal must support them and tmux must forward them (allow-passthrough on).
func Supported() bool {
	if os.Getenv("TMUX") != "" {
		passthrough := tmux("show", "-gv", "allow-passthrough")
		// The terminal name is often a generic xterm-256color; the type is what the
		// terminal reported about itself, e.g. "ghostty 1.4.0".
		return (passthrough == "on" || passthrough == "all") &&
			(kittyTerm(tmux("display", "-p", "#{client_termname}")) || kittyType(tmux("display", "-p", "#{client_termtype}")))
	}
	return kittyTerm(os.Getenv("TERM")) || os.Getenv("TERM_PROGRAM") == "ghostty" || os.Getenv("KITTY_WINDOW_ID") != ""
}

func kittyTerm(term string) bool { return term == "xterm-kitty" || term == "xterm-ghostty" }

func kittyType(termType string) bool {
	termType = strings.ToLower(termType)
	return strings.HasPrefix(termType, "ghostty") || strings.HasPrefix(termType, "kitty")
}

func tmux(args ...string) string {
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Transmit sends img to the terminal under id without showing it yet.
func Transmit(img image.Image, id int) (string, error) {
	var buf bytes.Buffer
	err := kitty.EncodeGraphics(&buf, img, &kitty.Options{
		Action:         kitty.Transmit,
		Quiet:          2, // no replies: they would arrive as key input
		ID:             id,
		Format:         kitty.PNG,
		Transmission:   kitty.Direct,
		Chunk:          true,
		ChunkFormatter: passthrough,
	})
	return buf.String(), err
}

// Place makes image id fill cols×rows cells wherever its placeholders appear,
// replacing an earlier size.
func Place(id, cols, rows int) string {
	return passthrough(ansi.KittyGraphics(nil, "a=d", "d=i", fmt.Sprintf("i=%d", id), "q=2")) +
		passthrough(ansi.KittyGraphics(nil, "a=p", "U=1", fmt.Sprintf("i=%d", id),
			fmt.Sprintf("c=%d", cols), fmt.Sprintf("r=%d", rows), "q=2"))
}

// Placeholders are the text rows that show image id at cols×rows cells: each cell is
// the placeholder character with diacritics for its row and column, in the image's color.
func Placeholders(id, cols, rows int) []string {
	lines := make([]string, rows)
	for r := range rows {
		var b strings.Builder
		fmt.Fprintf(&b, "\x1b[38;5;%dm", id)
		for c := range cols {
			b.WriteRune(kitty.Placeholder)
			b.WriteRune(kitty.Diacritic(r))
			b.WriteRune(kitty.Diacritic(c))
		}
		b.WriteString("\x1b[39m")
		lines[r] = b.String()
	}
	return lines
}

// IsPlaceholderRow reports whether a text row came from Placeholders.
func IsPlaceholderRow(line string) bool {
	return strings.HasPrefix(line, "\x1b[38;5;") && strings.ContainsRune(line, kitty.Placeholder)
}

// passthrough wraps a graphics sequence so tmux forwards it to the outer terminal
// instead of swallowing it; outside tmux it is sent as is.
func passthrough(seq string) string {
	if os.Getenv("TMUX") == "" {
		return seq
	}
	return "\x1bPtmux;" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
}
