package feed

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// articlePage is a typical news page: navigation, the article, and a footer.
const articlePage = `<!doctype html><html><head><title>Big news</title></head><body>
<nav><a href="/">Home</a> <a href="/about">About us</a> <a href="/subscribe">Subscribe now</a></nav>
<article><h1>Big news</h1>
<p>The first paragraph of the full article explains what happened, in enough words that a reader view recognises it as the main text of the page.</p>
<p>The second paragraph adds the details that the feed's short teaser left out, which is exactly why fetching the whole article is useful for readers.</p>
<p>A third paragraph finishes the story with a quote, a few numbers and a link to <a href="/more">more background</a> on the topic.</p>
</article>
<footer>Copyright Example News. Cookie settings. Privacy policy.</footer>
</body></html>`

func pageServer(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/story" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, articlePage)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func TestFullArticleKeepsTheArticleAndDropsTheRest(t *testing.T) {
	base := pageServer(t)
	html, err := FullArticle(context.Background(), base+"/story")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "The second paragraph adds the details") || !strings.Contains(html, "more background") {
		t.Fatalf("the article text is missing:\n%s", html)
	}
	for _, noise := range []string{"Subscribe now", "Cookie settings"} {
		if strings.Contains(html, noise) {
			t.Fatalf("%q from the menu or footer should be dropped:\n%s", noise, html)
		}
	}

	if _, err := FullArticle(context.Background(), base+"/missing"); err == nil {
		t.Fatal("a page that doesn't exist should fail")
	}
}
