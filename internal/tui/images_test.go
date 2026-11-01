package tui

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"alekohs/feedhatch/internal/graphics"
	"alekohs/feedhatch/internal/parser"
)

// imageServer serves one 1000x500 PNG and counts the requests.
func imageServer(t *testing.T) (url string, requests *int) {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1000, 500))); err != nil {
		t.Fatal(err)
	}
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(server.Close)
	return server.URL + "/chart.png", &count
}

// imageModel opens an article with one image; images are on when supported is true.
func imageModel(t *testing.T, src string, supported bool) (Model, tea.Cmd) {
	t.Helper()
	m := sizedModel(t)
	m.imagesSupported, m.imagesOn = supported, supported
	m.currentArticle().body = "Intro\n\n" + parser.Image + src + parser.ImageAlt + "A chart\n\nAfter"
	m = press(m, "alt+2")
	updated, cmd := m.Update(keyMessage("enter"))
	return updated.(Model), cmd
}

// run executes cmd and the commands of any batch it returns, collecting the messages.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, c := range batch {
			msgs = append(msgs, run(c)...)
		}
		return msgs
	}
	return []tea.Msg{msg}
}

func rawOutput(msgs []tea.Msg) string {
	var out strings.Builder
	for _, msg := range msgs {
		if raw, ok := msg.(tea.RawMsg); ok {
			out.WriteString(raw.Msg.(string))
		}
	}
	return out.String()
}

func placeholderRows(m Model) int {
	n := 0
	for _, line := range m.readerLines(m.readerWidth()) {
		if graphics.IsPlaceholderRow(line) {
			n++
		}
	}
	return n
}

func TestImageLoadsThenShowsAndFollowsTheReaderWidth(t *testing.T) {
	src, _ := imageServer(t)
	m, cmd := imageModel(t, src, true)

	if !strings.Contains(strings.Join(m.readerLines(m.readerWidth()), "\n"), "[Bild laddas…]") {
		t.Fatal("the reader should say the image is loading")
	}
	msgs := run(cmd)
	var loaded *imageLoadedMsg
	for _, msg := range msgs {
		if l, ok := msg.(imageLoadedMsg); ok {
			loaded = &l
		}
	}
	if loaded == nil || loaded.err != nil {
		t.Fatalf("opening the article should load its image, got %v", msgs)
	}

	updated, cmd := m.Update(*loaded)
	m = updated.(Model)
	out := rawOutput(run(cmd))
	if !strings.Contains(out, "\x1b_G") || !strings.Contains(out, "a=p") || !strings.Contains(out, "U=1") {
		t.Fatalf("a loaded image should be sent and placed, got %q", out)
	}
	cols, rows := imageSize(loaded.img, m.readerWidth())
	if got := placeholderRows(m); got != rows {
		t.Fatalf("got %d placeholder rows, want %d", got, rows)
	}

	updated, cmd = m.Update(tea.WindowSizeMsg{Width: 70, Height: 28})
	m = updated.(Model)
	newCols, _ := imageSize(loaded.img, m.readerWidth())
	out = rawOutput(run(cmd))
	if newCols == cols || !strings.Contains(out, "a=p") || strings.Contains(out, "f=100") {
		t.Fatalf("a narrower reader should place the image again without resending it (cols %d→%d): %q", cols, newCols, out)
	}

	text, _ := m.selectedText()
	if strings.ContainsRune(text, '\U0010EEEE') {
		t.Fatal("copying must not include placeholder characters")
	}
}

func TestUnsupportedTerminalNeverDownloads(t *testing.T) {
	src, requests := imageServer(t)
	m, cmd := imageModel(t, src, false)

	run(cmd)
	if *requests != 0 {
		t.Fatalf("got %d downloads, want none in a terminal without image support", *requests)
	}
	if !strings.Contains(strings.Join(m.readerLines(m.readerWidth()), "\n"), "[Bild: A chart]") {
		t.Fatal("the reader should show the alt text instead")
	}
	m = press(m, "I")
	if m.imagesOn || m.tone != toneWarn {
		t.Fatal("I should explain that the terminal can't show images")
	}
}

func TestImageToggle(t *testing.T) {
	src, _ := imageServer(t)
	m, _ := imageModel(t, src, true)

	m = press(m, "I")
	if m.imagesOn || !strings.Contains(strings.Join(m.readerLines(m.readerWidth()), "\n"), "[Bild: A chart]") {
		t.Fatal("I should turn images off and show the alt text")
	}
	updated, cmd := m.Update(keyMessage("j"))
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("nothing should load while images are off")
	}
	m = press(m, "I")
	if !m.imagesOn {
		t.Fatal("I should turn images on again")
	}
}

func TestResolveURL(t *testing.T) {
	cases := []struct{ link, src, want string }{
		{"https://blog.example/posts/a", "/img/x.png", "https://blog.example/img/x.png"},
		{"https://blog.example/posts/a/", "x.png", "https://blog.example/posts/a/x.png"},
		{"https://blog.example/posts/a", "https://cdn.example/y.png", "https://cdn.example/y.png"},
		{"", "/img/x.png", "/img/x.png"},
	}
	for _, c := range cases {
		if got := resolveURL(c.link, c.src); got != c.want {
			t.Errorf("resolveURL(%q, %q) = %q, want %q", c.link, c.src, got, c.want)
		}
	}
}

func TestImageSizeKeepsAspectAndLimits(t *testing.T) {
	wide := image.NewRGBA(image.Rect(0, 0, 1000, 500))
	if cols, rows := imageSize(wide, 60); cols != 60 || rows != 15 {
		t.Fatalf("1000x500 in 60 columns = %dx%d, want 60x15", cols, rows)
	}
	tall := image.NewRGBA(image.Rect(0, 0, 400, 2000))
	if cols, rows := imageSize(tall, 60); rows != maxImageRows || cols != 8 {
		t.Fatalf("400x2000 = %dx%d, want 8x%d", cols, rows, maxImageRows)
	}
}

func TestGalleryReusesIDsInACycle(t *testing.T) {
	g := newGallery()
	g.entries["first"] = &imageEntry{}
	if id := g.assignID("first"); id != graphics.FirstID {
		t.Fatalf("first id = %d, want %d", id, graphics.FirstID)
	}
	for i := graphics.FirstID + 1; i <= graphics.LastID; i++ {
		g.assignID("other")
	}
	if id := g.assignID("again"); id != graphics.FirstID {
		t.Fatalf("ids should wrap around to %d, got %d", graphics.FirstID, id)
	}
	if _, ok := g.entries["first"]; ok {
		t.Fatal("the image that held a reused id should be forgotten")
	}
}

func TestCoverImageComesFirstUnlessTheTextShowsIt(t *testing.T) {
	a := &article{body: "Text", image: "https://x.example/cover.png", link: "https://x.example/post"}
	lines := a.bodyLines()
	if src, _, ok := parser.ParseImage(lines[0]); !ok || src != "https://x.example/cover.png" || lines[2] != "Text" {
		t.Fatalf("lines = %q, want the cover image first", lines)
	}

	a.body = "Intro\n\n" + parser.Image + "/cover.png" + parser.ImageAlt + "\n\nText"
	if lines := a.bodyLines(); lines[0] != "Intro" {
		t.Fatalf("the cover is already in the text (as a relative URL), got %q", lines)
	}
}
