package images

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func serve(t *testing.T, files map[string][]byte) (*httptest.Server, *int) {
	t.Helper()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		data, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func TestLoadDownloadsOnceThenUsesTheCache(t *testing.T) {
	server, requests := serve(t, map[string][]byte{"/chart.png": pngBytes(t, 200, 100)})
	dir := t.TempDir()

	for range 2 {
		img, err := Load(server.URL+"/chart.png", dir)
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != 200 || b.Dy() != 100 {
			t.Fatalf("size = %v, want 200x100", b)
		}
	}
	if *requests != 1 {
		t.Fatalf("got %d downloads, want 1 thanks to the cache", *requests)
	}
}

func TestLoadScalesDownWideImages(t *testing.T) {
	server, _ := serve(t, map[string][]byte{"/wide.png": pngBytes(t, 3200, 800)})

	img, err := Load(server.URL+"/wide.png", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != maxWidth || b.Dy() != 400 {
		t.Fatalf("size = %v, want %dx400", b, maxWidth)
	}
}

func TestLoadRejectsTinyMissingAndBrokenImages(t *testing.T) {
	server, _ := serve(t, map[string][]byte{
		"/pixel.png": pngBytes(t, 1, 1),
		"/broken":    []byte("not an image"),
	})
	dir := t.TempDir()

	if _, err := Load(server.URL+"/pixel.png", dir); !errors.Is(err, ErrTiny) {
		t.Fatalf("err = %v, want ErrTiny for a tracking pixel", err)
	}
	if _, err := Load(server.URL+"/missing.png", dir); err == nil {
		t.Fatal("a 404 should fail")
	}
	if _, err := Load(server.URL+"/broken", dir); err == nil {
		t.Fatal("data that is not an image should fail")
	}
}

func TestPruneRemovesTheLeastRecentlyUsedImages(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	for i, name := range []string{"oldest", "older", "newer", "newest"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, make([]byte, 100), 0o600); err != nil {
			t.Fatal(err)
		}
		when := now.Add(time.Duration(i-4) * time.Hour)
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
	}

	if err := Prune(dir, 250); err != nil {
		t.Fatal(err)
	}
	var left []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if strings.Join(left, ",") != "newer,newest" {
		t.Fatalf("left = %v, want the two most recently used", left)
	}
	if err := Prune(filepath.Join(dir, "missing"), 1); err != nil {
		t.Fatalf("a cache that doesn't exist yet is fine: %v", err)
	}
}

func TestCacheHitMarksTheImageAsUsed(t *testing.T) {
	server, _ := serve(t, map[string][]byte{"/a.png": pngBytes(t, 20, 20)})
	dir := t.TempDir()
	if _, err := Load(server.URL+"/a.png", dir); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	path := filepath.Join(dir, entries[0].Name())
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(server.URL+"/a.png", dir); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(path); time.Since(info.ModTime()) > time.Minute {
		t.Fatal("reading a cached image should mark it as recently used")
	}
}

// gifBytes encodes frames of a 20x20 GIF; each frame fills rect with palette color c.
func gifBytes(t *testing.T, frames []image.Rectangle, colors []uint8, delays []int) []byte {
	t.Helper()
	palette := color.Palette{color.Transparent, color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}}
	g := &gif.GIF{Config: image.Config{Width: 20, Height: 20, ColorModel: palette}}
	for i, rect := range frames {
		frame := image.NewPaletted(rect, palette)
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			for x := rect.Min.X; x < rect.Max.X; x++ {
				frame.SetColorIndex(x, y, colors[i])
			}
		}
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, delays[i])
		g.Disposal = append(g.Disposal, gif.DisposalNone)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestLoadAnimatedGIF(t *testing.T) {
	full, corner := image.Rect(0, 0, 20, 20), image.Rect(0, 0, 5, 5)
	server, _ := serve(t, map[string][]byte{
		"/anim.gif":  gifBytes(t, []image.Rectangle{full, corner, corner}, []uint8{1, 2, 1}, []int{5, 0, 20}),
		"/still.gif": gifBytes(t, []image.Rectangle{full}, []uint8{1}, []int{0}),
	})
	dir := t.TempDir()

	img, err := Load(server.URL+"/anim.gif", dir)
	if err != nil {
		t.Fatal(err)
	}
	anim, ok := img.(*Animated)
	if !ok || len(anim.Frames) != 3 {
		t.Fatalf("got %T with %d frames, want an animation with 3", img, len(anim.Frames))
	}
	want := []time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond}
	if !slices.Equal(anim.Delays, want) {
		t.Fatalf("delays = %v, want %v (0 means 100ms, like browsers)", anim.Delays, want)
	}
	// The second frame only paints a blue corner over the red first frame.
	second := anim.Frames[1]
	if r, _, b, _ := second.At(2, 2).RGBA(); b == 0 || r != 0 {
		t.Fatal("the corner of frame 2 should be blue")
	}
	if r, _, _, _ := second.At(15, 15).RGBA(); r == 0 {
		t.Fatal("outside the corner, frame 2 should keep frame 1's red")
	}

	still, err := Load(server.URL+"/still.gif", dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := still.(*Animated); ok {
		t.Fatal("a one-frame GIF is a still image")
	}
}

func TestLongAnimationsAreCut(t *testing.T) {
	g := &gif.GIF{Config: image.Config{Width: 1000, Height: 1000}}
	palette := color.Palette{color.Black, color.White}
	for range 60 {
		g.Image = append(g.Image, image.NewPaletted(image.Rect(0, 0, 1000, 1000), palette))
		g.Delay = append(g.Delay, 5)
	}
	img, err := animation(g)
	if err != nil {
		t.Fatal(err)
	}
	if frames := len(img.(*Animated).Frames); frames != maxAnimationPixels/1_000_000 {
		t.Fatalf("kept %d frames, want %d within the pixel budget", frames, maxAnimationPixels/1_000_000)
	}
}
