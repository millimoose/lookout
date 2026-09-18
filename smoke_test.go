package main

import (
	"os"
	"testing"
)

// TestFixturePipeline is the smoke test: it drives the real example fixture
// through the whole non-TUI pipeline — load, column detection, filtering.
func TestFixturePipeline(t *testing.T) {
	f, err := os.Open("example.ndjson")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	entries, lines, err := ParseEntries(f)
	if err != nil {
		t.Fatalf("ParseEntries: %v", err)
	}
	if lines == 0 || len(entries) == 0 {
		t.Fatalf("fixture parsed empty: lines=%d entries=%d", lines, len(entries))
	}

	malformed := 0
	for _, e := range entries {
		if e.Malformed {
			malformed++
		}
	}
	if malformed != 2 {
		t.Errorf("malformed = %d, want 2 (the plain-text line and the truncated JSON line)", malformed)
	}

	cols := ColumnOrder(entries)
	if len(cols) == 0 {
		t.Fatal("no columns detected")
	}
	for _, want := range []string{"ts", "level", "service", "msg", "trace_id"} {
		found := false
		for _, c := range cols {
			if c == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("column %q missing from %v", want, cols)
		}
	}
	if cols[0] != "ts" || cols[1] != "level" || cols[2] != "service" || cols[3] != "msg" {
		t.Errorf("priority column order = %v, want ts, level, service, msg first", cols[:4])
	}

	// level=error keeps only error-level entries, all with the right field.
	shown := 0
	for _, e := range entries {
		if ParseFilter("level=error").Match(e) {
			shown++
			if e.Fields["level"] != "error" {
				t.Errorf("entry %d passed level=error filter with level=%q", e.LineNo, e.Fields["level"])
			}
		}
	}
	if shown == 0 {
		t.Error("level=error matched nothing in fixture")
	}

	// Substring filter matches raw text across any field.
	n := 0
	for _, e := range entries {
		if ParseFilter("payment").Match(e) {
			n++
		}
	}
	if n == 0 {
		t.Error("substring 'payment' matched nothing in fixture")
	}

	// Malformed entries never crash matching and are excluded by key filters.
	for _, e := range entries {
		if e.Malformed {
			if ParseFilter("level=info").Match(e) {
				t.Error("key filter matched a malformed entry")
			}
		}
	}
}
