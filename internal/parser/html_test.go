package parser

import (
	"slices"
	"testing"
)

func TestTitle(t *testing.T) {
	for src, want := range map[string]string{
		"The <script> tag explained": "The <script> tag explained",
		"Vector<int> tips":           "Vector<int> tips",
		"<b>Bold</b> &amp; plain":    "Bold & plain",
		"&lt;b&gt; is a tag":         "<b> is a tag",
		"  Two\n  lines ":            "Two lines",
	} {
		if got := Title(src); got != want {
			t.Errorf("Title(%q) = %q, want %q", src, got, want)
		}
	}
}

func TestText(t *testing.T) {
	cases := []struct {
		name, html, text string
		links            []string
	}{
		{
			name:  "paragraphs, links, entities and lists",
			html:  `<p>Go 1.27 is out. Read the <a href="https://go.dev/doc/go1.27">release notes</a>` + "\n" + `&amp; try the new <code>slices</code> helpers.</p><ul><li>Faster builds</li><li>New vet checks</li></ul>`,
			text:  "Go 1.27 is out. Read the " + LinkStart(1) + "release notes" + LinkOff + " & try the new slices helpers.\n\n• Faster builds\n• New vet checks",
			links: []string{"https://go.dev/doc/go1.27"},
		},
		{
			name:  "links are numbered in order",
			html:  `<p><a href="https://a.example">one</a> and <a href="https://b.example">two</a></p>`,
			text:  LinkStart(1) + "one" + LinkOff + " and " + LinkStart(2) + "two" + LinkOff,
			links: []string{"https://a.example", "https://b.example"},
		},
		{
			name:  "links around images or without text are dropped",
			html:  `<p><a href="https://a.example"><img src="https://a.example/i.png"></a><a href="https://b.example"> </a><a href="https://c.example">real</a></p>`,
			text:  Image + "https://a.example/i.png" + ImageAlt + "\n\n" + LinkStart(1) + "real" + LinkOff,
			links: []string{"https://c.example"},
		},
		{
			name:  "spaces next to markers are not doubled",
			html:  `<p>Read the <a href="https://a.example"> notes</a> and <b> this</b>.</p>`,
			text:  "Read the " + LinkStart(1) + "notes" + LinkOff + " and " + BoldOn + "this" + BoldOff + ".",
			links: []string{"https://a.example"},
		},
		{
			name: "headings keep their level",
			html: `<h1>Title</h1><p>Intro</p><h2>Section</h2><h3>Sub</h3><h6>Deep</h6>`,
			text: "# Title\n\nIntro\n\n## Section\n\n### Sub\n\n###### Deep",
		},
		{
			name: "heading text is trimmed",
			html: "<h2>\n  How many visitors are still vulnerable?\n</h2>",
			text: "## How many visitors are still vulnerable?",
		},
		{
			name: "strong and b are marked bold",
			html: `<p>This is <strong>important</strong> and <b>this too</b>.</p>`,
			text: "This is " + BoldOn + "important" + BoldOff + " and " + BoldOn + "this too" + BoldOff + ".",
		},
		{
			name: "em and i are marked italic, also nested in bold",
			html: `<p>An <em>emphasis</em>, an <i>idiom</i> and <strong>both <em>at once</em></strong>.</p>`,
			text: "An " + ItalicOn + "emphasis" + ItalicOff + ", an " + ItalicOn + "idiom" + ItalicOff + " and " +
				BoldOn + "both " + ItalicOn + "at once" + ItalicOff + BoldOff + ".",
		},
		{
			name: "literal asterisks and underscores stay text",
			html: `<p>Compute 2**8 in snake_case_name.</p>`,
			text: "Compute 2**8 in snake_case_name.",
		},
		{
			name: "preformatted text is kept",
			html: "<p>Code:</p><pre><code>func main() {\n\tfmt.Println(1)\n\n}\n</code></pre>",
			text: "Code:\n\n" + Code + "func main() {\n" + Code + "    fmt.Println(1)\n" + Code + "\n" + Code + "}",
		},
		{
			name: "scripts and styles are dropped, images get a line of their own",
			html: `<style>p{}</style><p>Chart: <img src="https://x.example/c.png" alt="Probes  per day"> done</p><script>track()</script>`,
			text: "Chart:\n\n" + Image + "https://x.example/c.png" + ImageAlt + "Probes per day\n\ndone",
		},
		{
			name: "tracking pixels are dropped, data images keep only their alt text",
			html: `<p>a<img src="https://t.example/p.gif" width="1" height="1">b<img src="data:image/png;base64,AAAA" alt="inline"></p>`,
			text: "ab[Bild: inline]",
		},
		{
			name: "plain text passes through",
			html: "Just a title & more",
			text: "Just a title & more",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, links := Text(c.html)
			if text != c.text {
				t.Errorf("text =\n%q\nwant\n%q", text, c.text)
			}
			if !slices.Equal(links, c.links) {
				t.Errorf("links = %q, want %q", links, c.links)
			}
		})
	}
}

func TestParseImage(t *testing.T) {
	src, alt, ok := ParseImage(Image + "https://x.example/c.png" + ImageAlt + "A chart")
	if !ok || src != "https://x.example/c.png" || alt != "A chart" {
		t.Fatalf("got %q %q %v", src, alt, ok)
	}
	if _, _, ok := ParseImage("plain text"); ok {
		t.Fatal("a text line is not an image")
	}
}

func TestPlainAndLinkNumber(t *testing.T) {
	marked := "a " + BoldOn + "b" + BoldOff + " " + LinkStart(12) + "c" + LinkOff + "\u00a0d"
	if got := Plain(marked); got != "a b c d" {
		t.Fatalf("Plain = %q", got)
	}
	if n, ok := LinkNumber([]rune(LinkStart(12))[0]); !ok || n != 12 {
		t.Fatalf("LinkNumber = %d, %v", n, ok)
	}
	if _, ok := LinkNumber('x'); ok {
		t.Fatal("x is no link marker")
	}
}
