package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const usage = `lv — NDJSON log file viewer

usage: lv [--tail] <file.ndjson>

keys:
  ↑/↓ j/k     scroll             /    filter (key=value or substring; space = AND)
  PgUp/PgDn   page               c    column picker
  g/G         top/bottom         f    toggle follow (tail -f)
  enter       entry detail       q    quit
`

func main() {
	var tail bool
	flag.BoolVar(&tail, "tail", false, "start in follow (tail -f) mode")
	flag.BoolVar(&tail, "f", false, "start in follow (tail -f) mode (shorthand)")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	path := flag.Arg(0)

	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lv:", err)
		os.Exit(1)
	}
	entries, lines, err := ParseEntries(f)
	f.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "lv:", err)
		os.Exit(1)
	}
	offset, err := LastLineEnd(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lv:", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	followCh := make(chan FollowEvent)
	go Follow(ctx, path, offset, 200*time.Millisecond, followCh)

	p := tea.NewProgram(NewModel(path, entries, lines, followCh, tail),
		tea.WithAltScreen(), tea.WithContext(ctx))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "lv:", err)
		os.Exit(1)
	}
}
