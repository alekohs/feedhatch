package images

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
