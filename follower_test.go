package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func recvEvent(t *testing.T, ch <-chan FollowEvent) FollowEvent {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("follow channel closed unexpectedly")
		}
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for follow event")
		return FollowEvent{}
	}
}

func TestFollow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.ndjson")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan FollowEvent)
	go Follow(ctx, path, 0, 5*time.Millisecond, ch)

	af, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer af.Close()

	// A partial line (no '\n') must not be emitted yet.
	if _, err := af.WriteString(`{"a":1`); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	select {
	case ev := <-ch:
		t.Fatalf("got event for partial line: %+v", ev)
	default:
	}

	// Completing the line emits exactly it.
	if _, err := af.WriteString("}\n"); err != nil {
		t.Fatal(err)
	}
	if ev := recvEvent(t, ch); len(ev.Lines) != 1 || ev.Lines[0] != `{"a":1}` {
		t.Fatalf("lines = %v, want [{\"a\":1}]", ev.Lines)
	}

	// A batch of lines arrives together.
	if _, err := af.WriteString("{\"b\":2}\n\n{\"c\":3}\n"); err != nil {
		t.Fatal(err)
	}
	if ev := recvEvent(t, ch); len(ev.Lines) != 3 || ev.Lines[0] != `{"b":2}` || ev.Lines[1] != "" || ev.Lines[2] != `{"c":3}` {
		t.Fatalf("lines = %q, want empty interior line preserved", ev.Lines)
	}

	// Cancellation closes the channel.
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel not closed after cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("channel not closed after cancel")
	}
}

func TestFollowDetectsTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.ndjson")
	if err := os.WriteFile(path, []byte("{\"a\":1}\n{\"b\":2}\n{\"c\":3}\n{\"d\":4}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan FollowEvent)
	go Follow(ctx, path, 30, 5*time.Millisecond, ch)

	// Shrink the file: the follower must report truncation and restart at 0,
	// re-emitting the remaining content from the top.
	if err := os.Truncate(path, 8); err != nil { // keep exactly the first line
		t.Fatal(err)
	}
	ev := recvEvent(t, ch)
	if !ev.Truncated {
		t.Fatalf("expected truncation event, got %+v", ev)
	}
	if len(ev.Lines) == 0 || ev.Lines[0] != `{"a":1}` {
		t.Fatalf("expected re-read from start, got %q", ev.Lines)
	}

	// Appends after truncation still flow.
	af, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer af.Close()
	if _, err := af.WriteString("{\"e\":5}\n"); err != nil {
		t.Fatal(err)
	}
	if ev := recvEvent(t, ch); len(ev.Lines) != 1 || ev.Lines[0] != `{"e":5}` {
		t.Fatalf("lines after truncation = %q", ev.Lines)
	}
}
