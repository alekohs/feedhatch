package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"alekohs/feedhatch/internal/store"
	"alekohs/feedhatch/internal/tui"
	"alekohs/feedhatch/internal/worker"
)

func main() {
	var daemon bool
	mock := flag.Bool("mock", false, "run with sample data in a temporary database")

	flag.BoolVar(&daemon, "daemon", false, "only fetch feeds on their intervals, without the TUI")
	flag.BoolVar(&daemon, "d", false, "only fetch feeds on their intervals, without the TUI")

	flag.Parse()

	store, err := openStore(*mock)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer store.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if daemon {
		log.Println("feedhatch: checking feeds every minute")
		worker.Run(ctx, store, log.Default(), func(error) {})
		return
	}

	model, err := tui.New(store)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	p := tea.NewProgram(model)
	// Mock feeds point at real URLs; fetching them would mix live articles into the sample data.
	if !*mock {
		// Log lines would draw over the TUI; errors reach the status bar through FetchedMsg instead.
		quiet := log.New(io.Discard, "", 0)
		go worker.Run(ctx, store, quiet, func(err error) { p.Send(tui.FetchedMsg{Err: err}) })
	}

	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func openStore(mock bool) (*store.Store, error) {
	if !mock {
		return store.Open()
	}

	return store.OpenMock()
}
