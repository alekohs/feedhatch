// Package images downloads and decodes article images, with a cache on disk so an
// image is only downloaded once.
package images

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	maxBytes = 20 << 20 // larger downloads are cut off and fail to decode
	maxWidth = 1600     // wider images are scaled down before they are sent to the terminal

	// MaxCache is how large the image cache may grow; Prune removes the least
	// recently used images beyond it.
	MaxCache = 200 << 20
)

// ErrTiny is returned for images too small to show, such as tracking pixels.
var ErrTiny = errors.New("image too small to show")

// Load returns the image at src, from the cache in dir when it was downloaded before.
func Load(src, dir string) (image.Image, error) {
	sum := sha256.Sum256([]byte(src))
	path := filepath.Join(dir, hex.EncodeToString(sum[:]))

	data, err := os.ReadFile(path)
	if err == nil {
		now := time.Now()
		_ = os.Chtimes(path, now, now) // marks the image as recently used for Prune
	} else {
		if data, err = download(src); err != nil {
			return nil, err
		}
		if err := save(path, data); err != nil {
			return nil, err
		}
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", src, err)
	}
	b := img.Bounds()
	if b.Dx() <= 2 || b.Dy() <= 2 {
		return nil, ErrTiny
	}
	if b.Dx() > maxWidth {
		scaled := image.NewRGBA(image.Rect(0, 0, maxWidth, b.Dy()*maxWidth/b.Dx()))
		draw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), img, b, draw.Src, nil)
		img = scaled
	}
	return img, nil
}

func download(src string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", src, res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, maxBytes))
}

// save writes the cache file through a temporary file, so a half-written download is
// never read back as an image.
func save(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "download-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Prune deletes the least recently used cached images in dir until the cache is at most
// limit bytes.
func Prune(dir string, limit int64) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	type file struct {
		path string
		size int64
		used time.Time
	}
	var files []file
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, file{filepath.Join(dir, e.Name()), info.Size(), info.ModTime()})
		total += info.Size()
	}
	slices.SortFunc(files, func(a, b file) int { return a.used.Compare(b.used) })
	for _, f := range files {
		if total <= limit {
			break
		}
		if err := os.Remove(f.path); err != nil {
			return err
		}
		total -= f.size
	}
	return nil
}
