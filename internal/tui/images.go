package tui

import (
	"errors"
	"image"
	"net/url"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"alekohs/feedhatch/internal/graphics"
	"alekohs/feedhatch/internal/images"
	"alekohs/feedhatch/internal/parser"
)

// maxImageRows keeps a tall image from filling the whole reader.
const maxImageRows = 20

// imageEntry is one article image: loading, failed, or decoded and sent to the terminal.
type imageEntry struct {
	img        image.Image
	err        error
	loading    bool
	id         int
	sent       bool
	cols, rows int // size of the terminal's current placement
}

// gallery holds the images of this session. It is shared between Model copies.
type gallery struct {
	entries map[string]*imageEntry
	byID    map[int]string
	nextID  int
}

func newGallery() *gallery {
	return &gallery{entries: map[string]*imageEntry{}, byID: map[int]string{}, nextID: graphics.FirstID}
}

// assignID gives src the next image ID. IDs are reused in a cycle; the image that held
// the ID before is forgotten and loaded again (from the disk cache) if it is shown later.
func (g *gallery) assignID(src string) int {
	id := g.nextID
	if old, ok := g.byID[id]; ok && old != src {
		delete(g.entries, old)
	}
	g.byID[id] = src
	g.nextID++
	if g.nextID > graphics.LastID {
		g.nextID = graphics.FirstID
	}
	return id
}

type imageLoadedMsg struct {
	src string
	img image.Image
	err error
}

func (m *Model) imageLoaded(msg imageLoadedMsg) {
	e := m.gallery.entries[msg.src]
	if e == nil {
		return
	}
	e.loading, e.img, e.err = false, msg.img, msg.err
	if e.img != nil {
		e.id = m.gallery.assignID(msg.src)
	}
}

// imageSize fits an image into the reader: about 10 pixels per column, at most width
// columns and maxImageRows rows, keeping the aspect ratio with cells twice as high as wide.
func imageSize(img image.Image, width int) (cols, rows int) {
	b := img.Bounds()
	cols = max(1, min(width, b.Dx()/10))
	rows = max(1, cols*b.Dy()/b.Dx()/2)
	if rows > maxImageRows {
		rows = maxImageRows
		cols = max(1, min(width, rows*2*b.Dx()/b.Dy()))
	}
	return cols, rows
}

// bodyLines are the article's text lines, starting with its cover image unless the
// text already shows that image.
func (a *article) bodyLines() []string {
	lines := strings.Split(a.body, "\n")
	if a.image == "" {
		return lines
	}
	for _, line := range lines {
		if src, _, ok := parser.ParseImage(line); ok && resolveURL(a.link, src) == resolveURL(a.link, a.image) {
			return lines
		}
	}
	return append([]string{parser.Image + a.image + parser.ImageAlt, ""}, lines...)
}

// resolveURL turns a relative image or link URL into an absolute one, relative to the article's link.
func resolveURL(link, src string) string {
	u, err := url.Parse(src)
	if err != nil || u.IsAbs() {
		return src
	}
	base, err := url.Parse(link)
	if err != nil || link == "" {
		return src
	}
	return base.ResolveReference(u).String()
}

// imageRows are the reader rows for an image line from the parser: the image itself
// when the terminal can show it, otherwise its alt text.
func (m Model) imageRows(line, link string, width int) []string {
	src, alt, _ := parser.ParseImage(line)
	fallback := "[Bild]"
	if alt != "" {
		fallback = "[Bild: " + alt + "]"
	}
	if !m.imagesOn {
		return strings.Split(ansi.Wrap(fallback, max(1, width), ""), "\n")
	}
	e := m.gallery.entries[resolveURL(link, src)]
	switch {
	case e == nil || e.loading:
		return []string{"[Bild laddas…]"}
	case errors.Is(e.err, images.ErrTiny):
		return nil
	case e.img == nil:
		return strings.Split(ansi.Wrap(fallback, max(1, width), ""), "\n")
	}
	cols, rows := imageSize(e.img, width)
	return graphics.Placeholders(e.id, cols, rows)
}

// imageCmds starts loading the images of the article on screen and sends loaded ones to
// the terminal, placed at the size the reader needs now.
func (m Model) imageCmds() tea.Cmd {
	if !m.imagesOn || !m.reading && !m.threeColumns() {
		return nil
	}
	a := m.currentArticle()
	if a == nil {
		return nil
	}
	width := m.readerWidth()
	var cmds []tea.Cmd
	for _, line := range a.bodyLines() {
		src, _, ok := parser.ParseImage(line)
		if !ok {
			continue
		}
		src = resolveURL(a.link, src)
		e := m.gallery.entries[src]
		if e == nil {
			m.gallery.entries[src] = &imageEntry{loading: true}
			dir := m.imageDir
			cmds = append(cmds, func() tea.Msg {
				img, err := images.Load(src, dir)
				return imageLoadedMsg{src: src, img: img, err: err}
			})
			continue
		}
		if e.img == nil {
			continue
		}
		var seq string
		if !e.sent {
			transmit, err := graphics.Transmit(e.img, e.id)
			if err != nil {
				e.img, e.err = nil, err
				continue
			}
			seq, e.sent = transmit, true
		}
		if cols, rows := imageSize(e.img, width); cols != e.cols || rows != e.rows {
			seq += graphics.Place(e.id, cols, rows)
			e.cols, e.rows = cols, rows
		}
		if seq != "" {
			cmds = append(cmds, tea.Raw(seq))
		}
	}
	return tea.Batch(cmds...)
}

// renderImageRow draws a row of image placeholders as they are: styling them would
// change the color that carries the image ID.
func renderImageRow(prefix, line string, width int, style lipgloss.Style) string {
	out := style.Render(prefix) + line
	if w := lipgloss.Width(out); w < width {
		out += style.Render(strings.Repeat(" ", width-w))
	}
	return out
}
