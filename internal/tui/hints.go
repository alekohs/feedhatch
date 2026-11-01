package tui

import "strings"

// hintKeys are the letters O labels links with, home row first.
const hintKeys = "asdfghjklqwertyuiopzxcvbnm"

// hintState is O's labelling of the links visible in the reader.
type hintState struct {
	labels map[int]string    // link number → label
	urls   map[string]string // label → URL
	typed  string
}

// hintLabels gives n distinct labels: single letters while they last, then pairs.
func hintLabels(n int) []string {
	var labels []string
	if n <= len(hintKeys) {
		for _, r := range hintKeys[:n] {
			labels = append(labels, string(r))
		}
		return labels
	}
	for _, a := range hintKeys {
		for _, b := range hintKeys {
			if len(labels) == n {
				return labels
			}
			labels = append(labels, string(a)+string(b))
		}
	}
	return labels
}

// startHints labels every link visible in the reader, in reading order.
func (m *Model) startHints() {
	lines := m.readerLines(m.readerWidth())
	_, _, height := m.dimensions()
	scroll := m.scroll
	if !m.reading {
		scroll = 0
	}
	start := min(scroll, max(0, len(lines)-(height-4)))
	var numbers []int
	seen := map[int]bool{}
	states := inlineAtStart(lines)
	for i := start; i < min(len(lines), start+height-4); i++ {
		state := states[i]
		if state.link > 0 && !seen[state.link] {
			seen[state.link] = true
			numbers = append(numbers, state.link)
		}
		for _, r := range lines[i] {
			if state.apply(r) && state.link > 0 && !seen[state.link] {
				seen[state.link] = true
				numbers = append(numbers, state.link)
			}
		}
	}
	links := m.articleLinks()
	if len(numbers) == 0 {
		m.status, m.tone = "Inga länkar syns i läsvyn.", toneWarn
		return
	}
	h := &hintState{labels: map[int]string{}, urls: map[string]string{}}
	for i, label := range hintLabels(len(numbers)) {
		if n := numbers[i]; n <= len(links) {
			h.labels[n], h.urls[label] = label, links[n-1]
		}
	}
	m.hints = h
	m.status, m.tone = "Skriv bokstäverna vid länken du vill öppna · esc avbryter", toneWarn
}

// typeHint handles a key while the links are labelled.
func (m *Model) typeHint(key string) {
	h := m.hints
	switch {
	case key == "esc":
		m.hints, m.status = nil, "Avbröts."
		return
	case key == "backspace":
		h.typed = h.typed[:max(0, len(h.typed)-1)]
	case len(key) == 1 && strings.Contains(hintKeys, key):
		h.typed += key
	default:
		m.hints, m.status = nil, "Avbröts."
		return
	}
	if url, ok := h.urls[h.typed]; ok {
		m.hints = nil
		m.openLink(url)
		return
	}
	for label := range h.urls {
		if strings.HasPrefix(label, h.typed) {
			m.status, m.tone = "Skriv bokstäverna vid länken du vill öppna · esc avbryter", toneWarn
			return
		}
	}
	m.hints, m.status = nil, "Ingen länk har de bokstäverna."
}
