# feedhatch
Hatch your feeds. Catch every update.

A keyboard-driven RSS reader built with Bubble Tea. Feeds, categories and
read state are saved in `~/.cache/feedhatch/feeds.db`. Each feed is fetched on
its own interval (default 15 minutes), checked once a minute.

```sh
make run       # your own feeds
make run-mock  # sample data in a temporary database, nothing is fetched
make run-headless  # only fetch feeds, no TUI; logs in the terminal
make test
```

## Install

Packages are attached to each [GitHub release](https://github.com/alekohs/feedhatch/releases).

```sh
curl -fsSL https://raw.githubusercontent.com/alekohs/feedhatch/main/install.sh | bash
```

The script detects the system and installs the latest release (or the one
given as an argument, e.g. `bash install.sh 0.2.0`), checking every download
against the release checksums:

| System | How |
| --- | --- |
| Arch and derivatives | builds `packaging/arch/PKGBUILD` with makepkg and installs it with pacman (run as your user) |
| Debian, Ubuntu and derivatives | installs the `.deb` with apt |
| Fedora, RHEL, openSUSE | installs the `.rpm` with dnf or zypper |
| macOS | the binary into `/usr/local/bin` |
| Other Linux | the binary into `~/.local/bin`, plus launcher entry, icon and user service under your home |

You can also download the packages yourself from the release.

The Linux packages install `feedhatch` with an app launcher entry: started from
the launcher it opens in your default terminal, typed in a terminal it runs
right there. To keep fetching while the TUI is closed:

```sh
systemctl --user enable --now feedhatch
```

`feedhatch -version` prints the installed version.

## Moving to another computer

```sh
feedhatch export feedhatch-backup.db   # on the old computer, also while it runs
feedhatch import feedhatch-backup.db   # on the new one
```

The export holds everything: feeds, categories, intervals, saved articles and
read state. Import checks that the file is a feedhatch export (and not from a
newer feedhatch), saves the current database as `feeds.db.bak-<time>` next to
it (keeping the three newest backups), and upgrades the imported copy to the current schema. Stop the daemon
(`systemctl --user stop feedhatch`) and close the TUI before importing; import
refuses to run while feedhatch is running. Add `-dev` to either command for the
development data.

## Releasing

Push a tag like `v0.2.0`; the release workflow runs the tests and GoReleaser,
which attaches tarballs, `.deb` and `.rpm` packages and checksums to the GitHub
release. `install.sh` always installs the latest release and sets the version in
`packaging/arch/PKGBUILD` itself, so the PKGBUILD needs no bump.
`make release-snapshot` builds everything locally into `dist/` without
publishing.

## Development

`-dev` (or `make run-dev` / `make run-headless-dev`) uses
`~/.cache/feedhatch-dev/` and `$XDG_RUNTIME_DIR/feedhatch-dev.sock` instead of
the real database and socket, so it can run next to your real feedhatch without
touching its data. `-mock` can't be combined with `-daemon`, since mock mode
never fetches.

## Logs

Every line shows the source file of the log call and its component:

```
12:19:01 INFO  <server/server.go:92> server: fetched feed feed="Arch Linux" new=3
12:19:02 WARN  <server/server.go:88> server: fetch failed feed=Example err="failed to detect feed type"
```

The daemon logs to stderr, with colored levels in a terminal and without the
time when the systemd journal adds its own. The TUI writes to `feedhatch.log`
in its data directory, so it never draws over the screen. The file is rotated
at 10 MB, keeping one old file (`-log-size` changes the limit, in MB):

```sh
tail -F ~/.cache/feedhatch/feedhatch.log
make dev-logs   # the same for -dev
```

Inside the TUI, `!` shows the same file.

## Running as a service

With a package installed, use `systemctl --user enable --now feedhatch` (see
Install). For development, `make install-service` builds feedhatch into `~/.local/bin`, installs
`init/feedhatch.service` as a systemd user service and starts it. It runs
`feedhatch -daemon` in the background, starts at login, and restarts if it
crashes. Run it again after pulling changes to update the binary.

```sh
make service-logs                 # follow the logs
systemctl --user stop feedhatch
systemctl --user disable feedhatch
```

## Requirements

- Go
- [golangci-lint](https://golangci-lint.run/) for `make lint` and `make package`

## Events

Whichever feedhatch runs the fetcher (the daemon, or the TUI when no daemon is
running) publishes events on `$XDG_RUNTIME_DIR/feedhatch.sock`, one JSON object
per line. The TUI uses them to reload, and other programs can listen too:

```sh
socat -u UNIX-CONNECT:$XDG_RUNTIME_DIR/feedhatch.sock - | jq .
```

```json
{"type":"feed","feed":"Arch Linux","new":3,"timestamp":"…"}
{"type":"error","feed":"Example","error":"failed to detect feed type","timestamp":"…"}
{"type":"round","timestamp":"…"}
```

Changes made in a TUI are announced the same way, so other TUIs reload and
scripts can react:

```json
{"type":"read","feed":"Arch Linux","article_id":12,"article":"…","timestamp":"…"}
{"type":"unread","feed":"Arch Linux","article_id":12,"article":"…","timestamp":"…"}
{"type":"feed_added","feed":"Example","category":"News","interval":15,"timestamp":"…"}
{"type":"feed_deleted","feed":"Example","timestamp":"…"}
{"type":"feed_read","feed":"Example","timestamp":"…"}
{"type":"category","feed":"Example","category":"Tech","timestamp":"…"}
{"type":"interval","feed":"Example","interval":30,"timestamp":"…"}
```

Clients can send these change events to the socket too; feedhatch passes them on
to every listener and ignores any other type.

If a daemon is already running, a TUI started afterwards does not fetch; it only
listens to the daemon's events. Whoever is only listening takes over fetching
within a couple of seconds when the fetching process stops, so a daemon started
while the TUI runs waits and carries on when you close the TUI.

## Keys

The sidebar starts with two lists across all feeds: **Olästa** (unread
articles, newest first) and **Alla artiklar**. feedhatch opens on Olästa when
something is unread. Articles you read there stay listed until you pick another
sidebar entry or press `r`, so the list doesn't jump while you read.

In terminals at least 140 columns wide the reader gets its own third column and
previews the selected article without marking it read; opening it with `enter`,
`l` or `3` marks it read and focuses the reader.

| Key | Action |
| --- | --- |
| `h` / `l` | Left/right panel; `l` also opens the selected article |
| `j` / `k` | Select feed or article; move the line cursor in the reader |
| `Enter` | Open feed/article |
| `Tab` | Switch panel |
| `Alt+1` / `Alt+2` / `Alt+3` | Focus the feed list / the article list / the reader |
| `5j`, `12k`, … | A count before a motion repeats it, like in Vim: `j`/`k`, `J`/`K`, `]`/`[`, `{`/`}` and the page keys; `20G` goes to line 20 |
| `]` / `[` | Open the next/previous unread article, continuing into the next feed |
| `J` / `K` | Next/previous article in the current list, also from the reader |
| `Esc` / `h` | Cancel selection, then back to the article list |
| `g` / `G` | First/last item or text line |
| `d` / `u`, `Ctrl+d` / `Ctrl+u` | Move half a page down/up |
| `f` / `b`, `Ctrl+f` / `Ctrl+b` | Move a full page down/up |
| `Space` / `PageDown`, `PageUp` | Move a full page down, or up with `PageUp`; at the end of an article `Space` opens the next unread |
| `{` / `}` | Previous/next category in the feed list; previous/next paragraph start in the reader |
| `H` / `M` / `L` | Move to the top/middle/bottom visible text row |
| `zz` | Center the current text row in the reader (clamped at article edges) |
| `/` | In the article list: filter the list live by title/text (`Enter` keeps the filter, `Esc` clears it); in the reader: search the current article (`Enter` searches, `Esc` cancels). Opening an article from a filtered list searches it for the filter text |
| `n` / `N` | Jump to the next/previous matching text row, wrapping at the edges |
| `v` | Start/stop line selection in the reader |
| `y` | Copy selected lines, or the current line if nothing is selected |
| `a` | Add feed: URL, optional name, category and update interval (default 15 minutes) |
| `c` | Move the selected feed to an existing or new category (not on Olästa/Alla artiklar) |
| `i` | Change how often the selected feed updates, in minutes |
| `A` | Mark all articles in the selected feed as read; on Olästa/Alla artiklar, all feeds after `y` |
| `D` | Delete the selected feed and its articles; `y` confirms, any other key cancels |
| `m` | Toggle read/unread for the selected article |
| `r` | Reload the lists from the database |
| `t` | Toggle terminal colors (default)/base theme |
| `I` | Show/hide images in articles (in terminals that can show them) |
| `o` | Open the link on the cursor line in your browser; with several links, the status bar numbers them and a digit picks one; on a line without links, the article itself. Clicking a link opens it too |
| `<` / `>` | Move the column border next to the focused panel; `=` resets the widths. You can also drag a border with the mouse |
| `?` | Show/close help |
| `!` | Show/close the log page (the end of `feedhatch.log`), re-read every second while open; at the end it follows new lines |
| `q` / `Ctrl+c` | Quit (`Ctrl+c` also works in forms) |

In forms, `Tab` switches field, `Enter` moves on/saves, `Ctrl+u` clears the
current field and `Esc` cancels. A new feed gets its articles on the next fetch round, within a minute.
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

## Links

Links in articles are underlined in color and open in your browser when clicked
(or with `o` on their line). Every article ends with a **Läs hela artikeln**
link, handy for feeds that only carry a teaser. Inside tmux, clicks need
`set -g mouse on`; terminals that support hyperlinks also make links
clickable themselves, which in tmux needs `set -as terminal-features ',*:hyperlinks'`.

## Images

In kitty and Ghostty, images in articles are shown in the reader (using the kitty
graphics protocol), including the cover image many feeds attach to each item.
They're downloaded when you open an article and cached in
`~/.cache/feedhatch/images`; tiny tracking pixels are skipped. `I` turns images
off and on. In other terminals feedhatch never downloads images and shows
`[Bild: <alt text>]` instead.

Inside tmux, images need passthrough (tmux 3.3 or newer):

```sh
set -g allow-passthrough on   # in ~/.tmux.conf
```

## Colors and clipboard

feedhatch starts in the terminal theme. The base theme (`t`) is dark with thin
borders and a teal focus accent. The terminal theme inherits the terminal's default background and text color and uses its
ANSI palette for accents. It therefore follows, for example, your Ghostty
configuration, even with a light theme. Focus is also shown with `>` and bold.

Copying uses **OSC52**, which is supported by Ghostty among others. The
terminal's settings (and any tmux/SSH configuration) must allow writing to the
clipboard. The app cannot confirm that the terminal received the text. Selection
is line-based and follows the text's visible line wraps; the selection is
cancelled when the terminal is resized to avoid copying the wrong lines.
