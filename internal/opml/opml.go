package opml

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strings"

	"golang.org/x/net/html/charset"

	"alekohs/feedhatch/internal/store"
)

type OPML struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr"`
	Head    Head     `xml:"head"`
	Body    Body     `xml:"body"`
}

type Head struct {
	Title string `xml:"title"`
}

type Body struct {
	Outlines []Outline `xml:"outline"`
}

type Outline struct {
	Type    string `xml:"type,attr,omitempty"`
	Text    string `xml:"text,attr"`
	Title   string `xml:"title,attr,omitempty"`
	XMLURL  string `xml:"xmlUrl,attr,omitempty"`
	HTMLURL string `xml:"htmlUrl,attr,omitempty"`

	Outlines []Outline `xml:"outline"`
}

func (o Outline) name() string {
	if o.Text != "" {
		return o.Text
	}
	return o.Title
}

// Parse reads an OPML document in any encoding it declares.
func Parse(r io.Reader) (OPML, error) {
	dec := xml.NewDecoder(r)
	dec.CharsetReader = charset.NewReaderLabel
	var opml OPML
	if err := dec.Decode(&opml); err != nil {
		return OPML{}, fmt.Errorf("not a valid OPML file: %w", err)
	}
	return opml, nil
}

// Write writes opml as an XML document.
func Write(w io.Writer, opml OPML) error {
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return enc.Encode(opml)
}

// Import adds the feeds in opml to s, each in the category of its nearest folder, and
// returns how many were added and skipped. Feeds already in s and addresses that aren't
// http(s) are skipped, so importing a file again is harmless.
func Import(opml OPML, s *store.Store) (added, skipped int, err error) {
	feeds, err := s.Feeds()
	if err != nil {
		return 0, 0, err
	}

	known := make(map[string]bool, len(feeds))
	for _, f := range feeds {
		known[f.URL] = true
	}

	var addOutlines func(outlines []Outline, category string) error
	addOutlines = func(outlines []Outline, category string) error {
		for _, o := range outlines {
			address := strings.TrimSpace(o.XMLURL)
			if address == "" {
				if err := addOutlines(o.Outlines, o.name()); err != nil {
					return err
				}
				continue
			}

			if known[address] || !validURL(address) {
				skipped++
				continue
			}
			feed := store.Feed{Name: o.name(), URL: address, Category: category}
			if _, err := s.AddFeed(feed); err != nil {
				return err
			}
			known[address] = true
			added++
		}
		return nil
	}
	err = addOutlines(opml.Body.Outlines, "")
	return added, skipped, err
}

func validURL(address string) bool {
	u, err := url.Parse(address)
	return err == nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https")
}

// Export returns the feeds in s, with one folder per category. OPML has no place for
// intervals, notifications or articles, so those are not included.
func Export(s *store.Store) (OPML, error) {
	feeds, err := s.Feeds()
	if err != nil {
		return OPML{}, err
	}

	opml := OPML{Version: "2.0", Head: Head{Title: "feedhatch"}}
	folders := map[string]int{} // category to its index in opml.Body.Outlines
	for _, f := range feeds {
		name := f.Name
		if name == "" {
			name = f.URL // text is required in OPML
		}
		outline := Outline{Type: "rss", Text: name, Title: name, XMLURL: f.URL}

		if f.Category == "" {
			opml.Body.Outlines = append(opml.Body.Outlines, outline)
			continue
		}
		i, ok := folders[f.Category]
		if !ok {
			i = len(opml.Body.Outlines)
			folders[f.Category] = i
			opml.Body.Outlines = append(opml.Body.Outlines, Outline{Text: f.Category})
		}
		opml.Body.Outlines[i].Outlines = append(opml.Body.Outlines[i].Outlines, outline)
	}

	return opml, nil
}
