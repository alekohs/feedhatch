package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"alekohs/feedhatch/internal/server"
	"alekohs/feedhatch/internal/tui"
)

func main() {
	var daemon bool

	mock := flag.Bool("mock", false, "run with sample data in a temporary database")
	flag.BoolVar(&daemon, "daemon", false, "only fetch feeds on their intervals, without the TUI")
	flag.BoolVar(&daemon, "d", false, "only fetch feeds on their intervals, without the TUI")
	flag.Parse()

	if daemon {
		runDaemon(*mock)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Log lines would draw over the TUI; errors reach the status bar through FetchedMsg instead.
	srv, err := server.Start(ctx, *mock, log.New(io.Discard, "", 0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer srv.Close()

	// The TUI talks to the socket whether this process or a daemon runs the worker:
	// it reloads on events and announces its own changes to other listeners.
	var updates chan tui.FetchedMsg
	var announce func(server.Event)
	if !*mock {
		updates = make(chan tui.FetchedMsg, 1)
		client := server.Connect(ctx, func(e server.Event) {
			if e.Type == "feed" || e.Type == "error" {
				return // a "round" event follows once all feeds are fetched
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

func runDaemon(mock bool) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv, err := server.Start(ctx, mock, log.Default())
	if err != nil {
		log.Fatal(err)
	}
	defer srv.Close()

	if !srv.Serving && !mock {
		log.Println(server.ErrRunning)
		return
	}

	log.Println("feedhatch: checking feeds every minute")
	<-ctx.Done()
}
