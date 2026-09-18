package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"time"
)

// FollowEvent is a batch of newly appended complete lines, or a truncation
// notice (Truncated, when the file shrank and reading restarted from the top).
type FollowEvent struct {
	Lines     []string
	Truncated bool
}

// Follow polls path for appended data every interval, sending each batch of
// complete lines on out (empty lines included, so line numbers stay true to
// the file). It closes out and returns when ctx is cancelled or the file
// disappears. A final line still missing its '\n' is held back until it
// completes.
func Follow(ctx context.Context, path string, startOffset int64, interval time.Duration, out chan<- FollowEvent) {
	defer close(out)
	offset := startOffset
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		ev, newOffset, alive := pollFile(path, offset)
		if !alive {
			return
		}
		offset = newOffset
		if len(ev.Lines) > 0 || ev.Truncated {
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}
}

func pollFile(path string, offset int64) (ev FollowEvent, newOffset int64, alive bool) {
	f, err := os.Open(path)
	if err != nil {
		return ev, offset, false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ev, offset, false
	}
	if st.Size() == offset {
		return ev, offset, true
	}
	if st.Size() < offset {
		// truncated or replaced: restart from the top
		offset = 0
		ev.Truncated = true
	}
	buf, err := io.ReadAll(io.NewSectionReader(f, offset, st.Size()-offset))
	if err != nil {
		return ev, offset, true
	}
	idx := bytes.LastIndexByte(buf, '\n')
	if idx < 0 {
		return ev, offset, true // only a partial line so far
	}
	newOffset = offset + int64(idx) + 1
	for _, ln := range strings.Split(strings.TrimSuffix(string(buf[:idx+1]), "\n"), "\n") {
		ev.Lines = append(ev.Lines, strings.TrimRight(ln, "\r"))
	}
	return ev, newOffset, true
}
