package parser

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// Inline style markers in Text's output, from Unicode's private use area so they never
// clash with article text. The reader draws the text between them and hides them.
const (
	BoldOn    = "\ue000"
	BoldOff   = "\ue001"
	ItalicOn  = "\ue002"
	ItalicOff = "\ue003"
)

// An image becomes a line of its own: Image, the src, ImageAlt and the alt text. The
// reader shows the image there, or the alt text when it can't.
const (
	Image    = "\ue004"
	ImageAlt = "\ue005"
)

// Code starts every line of a preformatted block; the reader draws those lines as a block.
const Code = "\ue006"

// Link text sits between LinkStart(n), for the nth of Text's links, and LinkOff.
const (
	LinkOff   = "\ue007"
	linkStart = 0xF0000 // a whole private use plane, one rune per link number
)

// LinkStart marks the start of the text of link n (counting from 1).
func LinkStart(n int) string { return string(rune(linkStart + n)) }

// LinkNumber returns n for a LinkStart(n) rune.
func LinkNumber(r rune) (int, bool) { return int(r - linkStart), r > linkStart && r <= 0xFFFFD }

func isMarker(r rune) bool { return r >= 0xE000 && r <= 0xE007 || r >= linkStart && r <= 0xFFFFD }

// Plain is text from Text without its markers, as search and copy should see it.
func Plain(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case isMarker(r):
			return -1
		case r == '\u00a0':
			return ' '
		}
		return r
	}, s)
}

// afterSpace reports whether the visible text so far ends a word, ignoring markers,
// so a following space can be dropped.
func afterSpace(s string) bool {
	for len(s) > 0 {
		r, size := utf8.DecodeLastRuneInString(s)
		if !isMarker(r) {
			return r == ' ' || r == '\n'
		}
		s = s[:len(s)-size]
	}
	return true
}

// ParseImage returns the src and alt text of an image line from Text.
func ParseImage(line string) (src, alt string, ok bool) {
	rest, ok := strings.CutPrefix(line, Image)
	if !ok {
		return "", "", false
	}
	src, alt, _ = strings.Cut(rest, ImageAlt)
	return src, alt, true
}

var (
	spaces     = regexp.MustCompile(`\s+`)
	blankLines = regexp.MustCompile(`\n{3,}`)
)

var titleTags = regexp.MustCompile(`(?i)</?(?:a|abbr|b|big|br|cite|code|del|em|font|i|ins|kbd|mark|q|s|small|span|strike|strong|sub|sup|u|var)\b[^>]*>`)

// Title turns a feed item's title into one line of plain text. Titles are mostly plain,
// so only inline formatting tags are removed and "The <script> tag" keeps its words.
func Title(src string) string {
	title := html.UnescapeString(titleTags.ReplaceAllString(src, ""))
	return strings.TrimSpace(spaces.ReplaceAllString(title, " "))
}

// Text turns an articles HTML into plain text for the reader. Links are numbered
// in the text, e.g. "release notes [1]", and returned in the same order
func Text(src string) (text string, links []string) {
	doc, err := html.Parse(strings.NewReader((src)))
	if err != nil {
		return src, nil
	}

	var b strings.Builder
	walk(doc, false, &b, &links)

	lines := strings.Split(b.String(), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}

	return strings.TrimSpace(blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")), links
}

func walk(n *html.Node, pre bool, b *strings.Builder, links *[]string) {
	linkAt := -1 // where this element's link text starts in b, if it is a link
	codeAt := -1 // where this pre element's text starts in b
	if n.Type == html.TextNode {
		t := n.Data
		if !pre {
			t = spaces.ReplaceAllString(t, " ")
			if afterSpace(b.String()) {
				t = strings.TrimLeft(t, " ")
			}
		}

		b.WriteString(t)
		return
	}

	if n.Type == html.ElementNode {
		switch n.Data {
		case "script", "style":
			return
		case "br":
			b.WriteString("\n")
			return
		case "img":
			src, alt := attr(n, "src"), strings.TrimSpace(spaces.ReplaceAllString(attr(n, "alt"), " "))
			switch {
			case attr(n, "width") == "1" || attr(n, "height") == "1":
				// tracking pixel
			case src == "" || strings.HasPrefix(src, "data:"):
				if alt != "" {
					b.WriteString("[Bild: " + alt + "]")
				}
			default:
				b.WriteString("\n\n" + Image + src + ImageAlt + alt + "\n\n")
			}
			return
		case "a":
			if href := attr(n, "href"); href != "" {
				*links = append(*links, href)
				linkAt = b.Len()
				b.WriteString(LinkStart(len(*links)))
			}
		case "strong", "b":
			b.WriteString(BoldOn)
		case "em", "i":
			b.WriteString(ItalicOn)
		case "li":
			b.WriteString("\n• ")
		case "h1", "h2", "h3", "h4", "h5", "h6":
			// Markdown-style markers keep the level visible; the reader styles them.
			b.WriteString("\n\n" + strings.Repeat("#", int(n.Data[1]-'0')) + " ")
		case "p", "div", "pre", "blockquote", "ul", "ol":
			b.WriteString("\n\n")
		}
		if n.Data == "pre" && !pre {
			codeAt = b.Len()
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, pre || n.Data == "pre", b, links)
	}

	if codeAt >= 0 {
		s := b.String()
		code := strings.Trim(strings.ReplaceAll(s[codeAt:], "\t", "    "), "\n")
		b.Reset()
		b.WriteString(s[:codeAt] + Code + strings.ReplaceAll(code, "\n", "\n"+Code))
	}

	if n.Type == html.ElementNode {
		switch n.Data {
		case "strong", "b":
			b.WriteString(BoldOff)
		case "em", "i":
			b.WriteString(ItalicOff)
		case "a":
			if linkAt >= 0 {
				s := b.String()
				text := s[linkAt+len(LinkStart(len(*links))):]
				if strings.TrimSpace(Plain(text)) == "" || strings.Contains(text, Image) {
					// Nothing to click on, e.g. a link around an image: drop the link.
					*links = (*links)[:len(*links)-1]
					b.Reset()
					b.WriteString(s[:linkAt] + text)
				} else {
					b.WriteString(LinkOff)
				}
			}
		case "p", "div", "pre", "blockquote", "ul", "ol", "h1", "h2", "h3", "h4", "h5", "h6":
			b.WriteString("\n\n")
		}
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}

	return ""
}
