# feedhatch
Hatch your feeds. Catch every update.

A keyboard-driven RSS reader built with Bubble Tea. The interface currently uses
mocked feeds and articles: no RSS is fetched and changes are not saved between
sessions.

```sh
go run ./cmd/feedhatch
go test ./...
```

## Requirements

- Go
- [golangci-lint](https://golangci-lint.run/) for `make lint` and `make package`

## Keys

| Key | Action |
| --- | --- |
| `h` / `l` | Left/right panel; `l` also opens the selected article |
| `j` / `k` | Select feed or article; move the line cursor in the reader |
| `Enter` | Open feed/article |
| `Tab` | Switch panel |
| `Esc` / `h` | Cancel selection, then back to the article list |
| `g` / `G` | First/last item or text line |
| `d` / `u`, `Ctrl+d` / `Ctrl+u` | Move half a page down/up |
| `f` / `b`, `Ctrl+f` / `Ctrl+b` | Move a full page down/up |
| `Space` / `PageDown`, `PageUp` | Move a full page down, or up with `PageUp` |
| `{` / `}` | Jump to the previous/next paragraph start in the reader |
| `H` / `M` / `L` | Move to the top/middle/bottom visible text row |
| `zz` | Center the current text row in the reader (clamped at article edges) |
| `/` | Enter a text search in the current article; `Enter` searches, `Esc` cancels |
| `n` / `N` | Jump to the next/previous matching text row, wrapping at the edges |
| `v` | Start/stop line selection in the reader |
| `y` | Copy selected lines, or the current line if nothing is selected |
| `a` | Add feed: URL, optional name and category |
| `c` | Move the selected feed to an existing or new category |
| `m` | Toggle read/unread for the selected article |
| `t` | Toggle base theme/terminal colors |
| `?` | Show/close help |
| `q` / `Ctrl+c` | Quit (`Ctrl+c` also works in forms) |

In forms, `Tab` switches field, `Enter` moves on/saves, `Ctrl+u` clears the
current field and `Esc` cancels. A new feed has no articles in mock mode.
Articles are marked as read when opened. `●` indicates unread content.

Reader motions also extend an active `v` selection. For example, `v` then `}`
selects through the next paragraph start, and `y` copies those rows. Paragraphs
are separated by blank lines; wrapped rows within a paragraph do not count as
new paragraphs. `{` first returns to the current paragraph's start when inside
it, then moves to the previous paragraph on the next press. Page motions also
work in lists and help; in lists they move by the corresponding number of items.
The existing single-key `g` shortcut remains available (so `gg` also reaches
the beginning).

Search is literal and case-insensitive, with highlighted matches. It searches
the current article's rendered rows (including its title and metadata), not all
feeds. A query must fit within one rendered row; it does not span a visual line
wrap. Multiple occurrences on the same row count as one navigation stop.
`n` and `N` also extend a visual selection. Submitting an empty `/` prompt
repeats the previous search; `Ctrl+u` clears the input. Cancelling the prompt
keeps the previous query and cursor position. `?` still opens help.

## Colors and clipboard

The base theme is dark with thin borders and a teal focus accent. The terminal
theme inherits the terminal's default background and text color and uses its
ANSI palette for accents. It therefore follows, for example, your Ghostty
configuration, even with a light theme. Focus is also shown with `>` and bold.

Copying uses **OSC52**, which is supported by Ghostty among others. The
terminal's settings (and any tmux/SSH configuration) must allow writing to the
clipboard. The app cannot confirm that the terminal received the text. Selection
is line-based and follows the text's visible line wraps; the selection is
cancelled when the terminal is resized to avoid copying the wrong lines.
