package main

import (
	"strings"
	"testing"
)

func TestParseEntries(t *testing.T) {
	in := `{"level":"info","msg":"ok"}

{"level":"error","msg":"boom","n":3,"ok":true,"nullv":null,"nested":{"a":1}}
not json at all
{"ts":"2026-09-18T08:00:01Z","level":"debug"}
`
	entries, lines, err := ParseEntries(strings.NewReader(in))
	if err != nil {
		t.Fatalf("ParseEntries: %v", err)
	}
	if want := 5; lines != want {
		t.Errorf("lines = %d, want %d", lines, want)
	}
	if len(entries) != 4 {
		t.Fatalf("len(entries) = %d, want 4", len(entries))
	}

	// line 1
	e := entries[0]
	if e.LineNo != 1 || e.Malformed {
		t.Errorf("entry0: LineNo=%d Malformed=%v", e.LineNo, e.Malformed)
	}
	if e.Fields["level"] != "info" || e.Fields["msg"] != "ok" {
		t.Errorf("entry0 fields = %v", e.Fields)
	}

	// line 3: value stringification
	e = entries[1]
	if got := e.Fields["n"]; got != "3" {
		t.Errorf("n = %q, want \"3\"", got)
	}
	if got := e.Fields["ok"]; got != "true" {
		t.Errorf("ok = %q, want \"true\"", got)
	}
	if got := e.Fields["nullv"]; got != "null" {
		t.Errorf("nullv = %q, want \"null\"", got)
	}
	if got := e.Fields["nested"]; got != `{"a":1}` {
		t.Errorf("nested = %q, want compact JSON", got)
	}

	// line 4: malformed, carries error, no fields
	e = entries[2]
	if !e.Malformed || e.ParseErr == "" || e.Fields != nil {
		t.Errorf("malformed entry = %+v", e)
	}
	if e.LineNo != 4 {
		t.Errorf("malformed LineNo = %d, want 4", e.LineNo)
	}
}

func TestParseLineLiteralNull(t *testing.T) {
	e := ParseLine(1, "null")
	if !e.Malformed {
		t.Error("literal null should be malformed (not an object)")
	}
}

func TestColumnOrder(t *testing.T) {
	entries := []Entry{
		{Fields: map[string]string{"msg": "a", "zeta": "z", "ts": "t", "level": "l"}},
		{Fields: map[string]string{"alpha": "x", "msg": "b"}},
		{}, // malformed/empty entries contribute nothing
	}
	got := ColumnOrder(entries)
	want := []string{"ts", "level", "msg", "zeta", "alpha"}
	if len(got) != len(want) {
		t.Fatalf("ColumnOrder = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ColumnOrder = %v, want %v", got, want)
		}
	}
}
