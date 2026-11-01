package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/log/v2"
	"gopkg.in/natefinch/lumberjack.v2"

	"alekohs/feedhatch/internal/constants"
	"alekohs/feedhatch/internal/event"
	"alekohs/feedhatch/internal/graphics"
	"alekohs/feedhatch/internal/images"
	"alekohs/feedhatch/internal/server"
	"alekohs/feedhatch/internal/settings"
	"alekohs/feedhatch/internal/store"
	"alekohs/feedhatch/internal/tui"
)

// version is set at build time, e.g. -ldflags "-X main.version=1.0.0".
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "export" || os.Args[1] == "import") {
		runTransfer(os.Args[1], os.Args[2:])
		return
	}

	var daemon bool

	mock := flag.Bool("mock", false, "run with sample data in a temporary database")
	dev := flag.Bool("dev", false, "use a separate development database and socket (feedhatch-dev)")
	logSize := flag.Int("log-size", 10, "rotate the TUI's log file at this size in MB, keeping one old file")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.BoolVar(&daemon, "daemon", false, "only fetch feeds on their intervals, without the TUI")
	flag.BoolVar(&daemon, "d", false, "only fetch feeds on their intervals, without the TUI")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: feedhatch [flags]\n       feedhatch export|import [-dev] <file>")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Println("feedhatch", version)
		return
	}

	if daemon && *mock {
		fmt.Fprintln(os.Stderr, "-mock can't be combined with -daemon: mock mode never fetches")
		os.Exit(2)
	}

	name := "feedhatch"
	if *dev {
		name = "feedhatch-dev"
	}

	if daemon {
		runDaemon(name)
		return
	}
	runTUI(name, *mock, *logSize)
}

func runTUI(name string, mock bool, logSize int) {
	logger, logPath, logFile, err := tuiLogger(name, logSize)
	if err != nil {
		fail(err)
	}
	defer logFile.Close()

	if !mock {
		if err := store.CreateWithExamples(name); err != nil {
			fail(err)
		}
	}
	settingsPath, err := settingsPathFor(name)
	if err != nil {
		fail(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := server.Start(ctx, server.Options{Name: name, Mock: mock, Logger: logger, SettingsPath: settingsPath})
	if err != nil {
		fail(err)
	}
	defer srv.Close()

	var updates chan tui.FetchedMsg
	var announce func(event.Event)
	if !mock {
		updates, announce = connectTUI(ctx, name)
	}

	imageDir := filepath.Join(filepath.Dir(logPath), "images")
	go pruneImages(imageDir, logger)

	model, err := tui.New(srv.Store, updates, announce, tui.Config{
		Mock:         mock,
		LogPath:      logPath,
		ImageDir:     imageDir,
		SettingsPath: settingsPath,
		Images:       graphics.Supported(),
	})
	if err != nil {
		fail(err)
	}
	if _, err := tea.NewProgram(model).Run(); err != nil {
		fail(err)
	}
}

// tuiLogger logs to a file, since log lines would draw over the TUI; errors also reach
// the status bar. The file is rotated at logSize MB with one old file kept.
func tuiLogger(name string, logSize int) (*slog.Logger, string, io.Closer, error) {
	logPath, err := logFilePath(name)
	if err != nil {
		return nil, "", nil, err
	}
	logFile := &lumberjack.Logger{Filename: logPath, MaxSize: logSize, MaxBackups: 1}
	return newLogger(logFile, time.DateTime), logPath, logFile, nil
}

// settingsPathFor is the settings file of the feedhatch called name, in the user's
// config directory (e.g. ~/.config/feedhatch/settings.json), created on first start.
// The TUI and the daemon share it, so Alt+b in the TUI also silences the daemon.
func settingsPathFor(name string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name, constants.SettingsFile)
	return path, settings.Ensure(path)
}

// connectTUI connects to the socket, whether this process or a daemon runs the worker:
// events arrive on updates so the TUI reloads, and announce tells other listeners about
// the TUI's own changes.
func connectTUI(ctx context.Context, name string) (updates chan tui.FetchedMsg, announce func(event.Event)) {
	updates = make(chan tui.FetchedMsg, 1)
	client := server.Connect(ctx, name, func(e event.Event) {
		if e.Type == event.Fetched || e.Type == event.Failed {
			return // an event.Round follows once all feeds are fetched
		}
		msg := tui.FetchedMsg{}
		if e.Error != "" {
			msg.Err = errors.New(e.Error)
		}
		select {
		case updates <- msg:
		default: // a reload is already pending
		}
	})
	return updates, client.Send
}

func pruneImages(dir string, logger *slog.Logger) {
	if err := images.Prune(dir, images.MaxCache); err != nil {
		logger.Warn("could not prune the image cache", "err", err)
	}
}

// runTransfer copies the database to or from a file, e.g. to move feedhatch to another computer.
func runTransfer(cmd string, args []string) {
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	dev := fs.Bool("dev", false, "use the development database (feedhatch-dev)")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: feedhatch %s [-dev] <file>\n", cmd)
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}

	name := "feedhatch"
	if *dev {
		name = "feedhatch-dev"
	}
	dir, err := store.Dir(name)
	if err != nil {
		fail(err)
	}
	db, file := filepath.Join(dir, constants.FeedDb), fs.Arg(0)

	switch cmd {
	case "export":
		if _, err := os.Stat(db); err != nil {
			fail(fmt.Errorf("no database to export at %s", db))
		}
		st, err := store.Open(name)
		if err != nil {
			fail(err)
		}
		defer st.Close()
		if err := st.Export(file); err != nil {
			fail(err)
		}
		fmt.Printf("Exported %s to %s\n", db, file)
	case "import":
		if server.Running(name) {
			fail(errors.New("feedhatch is running: stop the daemon (systemctl --user stop feedhatch) and close the TUI first"))
		}
		backup, err := store.Import(file, db)
		if backup != "" {
			fmt.Printf("Saved the previous database as %s\n", backup)
		}
		if err != nil {
			fail(err)
		}
		fmt.Printf("Imported %s into %s\n", file, db)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func runDaemon(name string) {
	logger := newLogger(os.Stderr, time.TimeOnly)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := store.CreateWithExamples(name); err != nil {
		logger.Error("could not create the database", "err", err)
		os.Exit(1)
	}
	settingsPath, err := settingsPathFor(name)
	if err != nil {
		logger.Error("could not start", "err", err)
		os.Exit(1)
	}
	srv, err := server.Start(ctx, server.Options{Name: name, Logger: logger, SettingsPath: settingsPath})
	if err != nil {
		logger.Error("could not start", "err", err)
		os.Exit(1)
	}
	defer srv.Close()

	if srv.Serving() {
		logger.Info("checking feeds every minute")
	} else {
		logger.Info("another feedhatch is fetching; taking over when it stops")
	}
	<-ctx.Done()
}

// newLogger writes one line per entry with the source file of the log call, e.g.
// "12:00:05 INFO <server/server.go:92> server: fetched feed feed=\"Arch Linux\" new=3".
// Levels are colored on a terminal only. Under systemd the journal adds its own time.
func newLogger(w io.Writer, timeFormat string) *slog.Logger {
	l := log.NewWithOptions(w, log.Options{
		ReportTimestamp: os.Getenv("JOURNAL_STREAM") == "",
		TimeFormat:      timeFormat,
		ReportCaller:    true,
	})
	return slog.New(componentHandler{l})
}

// componentHandler shows a logger's "component" attribute as the line prefix ("server:")
// instead of a trailing component=server field.
type componentHandler struct{ *log.Logger }

func (h componentHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	l := h.Logger
	var fields []any
	for _, a := range attrs {
		if a.Key == "component" {
			l = l.WithPrefix(a.Value.String())
			continue
		}
		fields = append(fields, a.Key, a.Value)
	}
	if len(fields) > 0 {
		l = l.With(fields...)
	}
	return componentHandler{l}
}

// logFilePath is the TUI's log file, rotated at -log-size MB with one old file kept.
func logFilePath(name string) (string, error) {
	dir, err := store.Dir(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, constants.LogFile), nil
}
