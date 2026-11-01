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

	"alekohs/feedhatch/internal/event"
	"alekohs/feedhatch/internal/server"
	"alekohs/feedhatch/internal/store"
	"alekohs/feedhatch/internal/tui"
)

func main() {
	var daemon bool

	mock := flag.Bool("mock", false, "run with sample data in a temporary database")
	dev := flag.Bool("dev", false, "use a separate development database and socket (feedhatch-dev)")
	logSize := flag.Int("log-size", 10, "rotate the TUI's log file at this size in MB, keeping one old file")
	flag.BoolVar(&daemon, "daemon", false, "only fetch feeds on their intervals, without the TUI")
	flag.BoolVar(&daemon, "d", false, "only fetch feeds on their intervals, without the TUI")
	flag.Parse()

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
	// Log lines would draw over the TUI, so they go to a file; errors also reach the status bar.
	logFile, err := openLogFile(name, logSize)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer logFile.Close()
	logger := newLogger(logFile, time.DateTime)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := server.Start(ctx, server.Options{Name: name, Mock: mock, Logger: logger})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer srv.Close()

	// The TUI talks to the socket whether this process or a daemon runs the worker:
	// it reloads on events and announces its own changes to other listeners.
	var updates chan tui.FetchedMsg
	var announce func(event.Event)
	if !mock {
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
		announce = client.Send
	}

	model, err := tui.New(srv.Store, updates, announce)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runDaemon(name string) {
	logger := newLogger(os.Stderr, time.TimeOnly)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv, err := server.Start(ctx, server.Options{Name: name, Logger: logger})
	if err != nil {
		logger.Error("could not start", "err", err)
		os.Exit(1)
	}
	defer srv.Close()

	if !srv.Serving {
		logger.Error(server.ErrRunning.Error())
		return
	}

	logger.Info("checking feeds every minute")
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

// openLogFile returns the TUI's log file, rotated at maxSize MB with one old file kept.
func openLogFile(name string, maxSize int) (io.WriteCloser, error) {
	dir, err := store.Dir(name)
	if err != nil {
		return nil, err
	}
	return &lumberjack.Logger{Filename: filepath.Join(dir, "feedhatch.log"), MaxSize: maxSize, MaxBackups: 1}, nil
}
