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
	"image/gif"
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

	"alekohs/feedhatch/internal/constants"
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

	if bytes.HasPrefix(data, []byte("GIF")) {
		if g, err := gif.DecodeAll(bytes.NewReader(data)); err == nil && len(g.Image) > 1 {
			return animation(g)
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
	return fit(img), nil
}

// fit scales an image down to maxWidth.
func fit(img image.Image) image.Image {
	b := img.Bounds()
	if b.Dx() <= maxWidth {
		return img
	}
	scaled := image.NewRGBA(image.Rect(0, 0, maxWidth, b.Dy()*maxWidth/b.Dx()))
	draw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), img, b, draw.Src, nil)
	return scaled
}

// Animated is an animated image. It is its first frame, so code that shows still
// images shows that; Frames are all frames, each shown for its Delay.
type Animated struct {
	image.Image
	Frames []image.Image
	Delays []time.Duration
}

// maxAnimationPixels caps an animation's frames, so a large or long GIF can't use
// hundreds of megabytes; frames beyond it are dropped.
const maxAnimationPixels = 40_000_000

// animation turns a GIF's frames, which often only update part of the picture, into
// complete frames.
func animation(g *gif.GIF) (image.Image, error) {
	bounds := image.Rect(0, 0, g.Config.Width, g.Config.Height)
	if bounds.Dx() <= 2 || bounds.Dy() <= 2 {
		return nil, ErrTiny
	}
	canvas := image.NewRGBA(bounds)
	a := &Animated{}
	pixels := 0
	for i, frame := range g.Image {
		disposal := byte(0)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}
		var previous *image.RGBA
		if disposal == gif.DisposalPrevious {
			previous = image.NewRGBA(bounds)
			draw.Draw(previous, bounds, canvas, image.Point{}, draw.Src)
		}
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)

		full := image.NewRGBA(bounds)
		draw.Draw(full, bounds, canvas, image.Point{}, draw.Src)
		scaled := fit(full)
		if pixels += scaled.Bounds().Dx() * scaled.Bounds().Dy(); pixels > maxAnimationPixels && len(a.Frames) > 0 {
			break
		}
		a.Frames = append(a.Frames, scaled)
		delay := 100 * time.Millisecond // browsers' choice for GIFs without a usable delay
		if i < len(g.Delay) && g.Delay[i] > 1 {
			delay = time.Duration(g.Delay[i]) * 10 * time.Millisecond
		}
		a.Delays = append(a.Delays, delay)

		switch disposal {
		case gif.DisposalBackground:
			draw.Draw(canvas, frame.Bounds(), image.Transparent, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			canvas = previous
		}
	}
	a.Image = a.Frames[0]
	return a, nil
}

func download(src string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", constants.UserAgent)
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
