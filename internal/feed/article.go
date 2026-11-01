package feed

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"codeberg.org/readeck/go-readability/v2"
)

// FullArticle downloads the page at link and returns its main content as HTML, without
// menus, ads and footers, for feeds that only carry a teaser.
func FullArticle(ctx context.Context, link string) (string, error) {
	pageURL, err := url.Parse(link)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return "", err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: %s", link, res.Status)
	}

	article, err := readability.FromReader(res.Body, pageURL)
	if err != nil {
		return "", fmt.Errorf("find the article in %s: %w", link, err)
	}
	var html bytes.Buffer
	if err := article.RenderHTML(&html); err != nil {
		return "", err
	}
	return html.String(), nil
}
