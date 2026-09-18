package main

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const testLogs = `{"ts":"2026-09-18T08:00:01Z","level":"info","service":"auth-svc","msg":"listening on port 8080"}
{"ts":"2026-09-18T08:00:02Z","level":"error","service":"payment-svc","msg":"card charge declined"}
{"ts":"2026-09-18T08:00:03Z","level":"warn","service":"api-gateway","msg":"slow upstream"}
worker crashed
`

func newTestModel(t *testing.T) Model {
	t.Helper()
	entries, lines, err := ParseEntries(strings.NewReader(testLogs))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel("test.ndjson", entries, lines, nil, false)
	m.width, m.height = 100, 12
	return m
}

// Sending a key message through Update must return a *copy* with the mode
// change; drive the real Update path rather than setting fields by hand.
func sendKey(t *testing.T, m Model, key string) Model {
	t.Helper()
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return m2.(Model)
}

func TestViewRendersAllEntries(t *testing.T) {
	m := newTestModel(t)
	v := m.View()
	for _, want := range []string{"listening on port 8080", "card charge declined", "slow upstream", "invalid JSON"} {
		if !strings.Contains(v, want) {
			t.Errorf("table view missing %q", want)
		}
	}
}

func TestFilterReducesRows(t *testing.T) {
	m := newTestModel(t)
	m = sendKey(t, m, "/")
	for _, ch := range "level=error" {
		m = sendKey(t, m, string(ch))
	}
	m = sendKey(t, m, "enter")

	if got := len(m.visible); got != 1 {
		t.Fatalf("visible = %d, want 1 (the error entry)", got)
	}
	v := m.View()
	if !strings.Contains(v, "card charge declined") {
		t.Error("filtered view lost the error entry")
	}
	for _, gone := range []string{"listening on port 8080", "slow upstream"} {
		if strings.Contains(v, gone) {
			t.Errorf("filtered view still shows %q", gone)
		}
	}
	if !strings.Contains(v, "filter: level=error") {
		t.Error("status bar does not show the active filter")
	}
}

func TestColumnPickerHidesColumn(t *testing.T) {
	m := newTestModel(t)
	if !strings.Contains(m.View(), "payment-svc") {
		t.Fatal("precondition: service column visible")
	}
	m = sendKey(t, m, "c")
	// columns: ts, level, service, msg (+trace-less extras); service is 3rd
	m = sendKey(t, m, "down") // -> level
	m = sendKey(t, m, "down") // -> service
	m = sendKey(t, m, " ")    // hide service
	m = sendKey(t, m, "esc")

	v := m.View()
	if strings.Contains(v, "payment-svc") {
		t.Error("service column still rendered after hiding")
	}
	if !strings.Contains(v, "card charge declined") {
		t.Error("msg column lost after hiding service")
	}
}

func TestFollowSticksToBottomOnAppend(t *testing.T) {
	entries, lines, _ := ParseEntries(strings.NewReader(testLogs))
	m := NewModel("test.ndjson", entries, lines, nil, true)
	m.width, m.height = 100, 6 // rowsAvail = 3
	m2, _ := m.Update(linesMsg{Lines: []string{
		`{"ts":"2026-09-18T08:00:04Z","level":"info","service":"auth-svc","msg":"appended line one"}`,
		`{"ts":"2026-09-18T08:00:05Z","level":"info","service":"auth-svc","msg":"appended line two"}`,
	}})
	mm := m2.(Model)
	if mm.cursor != len(mm.visible)-1 {
		t.Errorf("cursor = %d, want %d (last row) in follow mode", mm.cursor, len(mm.visible)-1)
	}
	v := mm.View()
	if !strings.Contains(v, "appended line two") {
		t.Error("follow mode did not scroll to the newly appended entry")
	}
}

func TestDetailShowsPrettyNestedJSON(t *testing.T) {
	entries, lines, _ := ParseEntries(strings.NewReader(
		`{"level":"error","msg":"boom","attrs":{"k":"v","n":1}}` + "\n"))
	m := NewModel("t", entries, lines, nil, false)
	m.width, m.height = 80, 20
	m = sendKey(t, m, "enter")
	v := m.View()
	for _, want := range []string{"attrs:", `"k": "v"`, `"n": 1`} {
		if !strings.Contains(v, want) {
			t.Errorf("detail view missing %q", want)
		}
	}
}

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func plain(s string) string { return ansiRe.ReplaceAllString(s, "") }

// TestHeaderAlignsWithRows guards the column offset bug where the header's
// first cell started one column left of the row cells.
func TestHeaderAlignsWithRows(t *testing.T) {
	m := newTestModel(t)
	m.width = 200
	hdr := plain(m.headerLine())
	row := plain(m.rowLine(0))
	hi, ri := strings.Index(hdr, "ts"), strings.Index(row, "2026-09-18")
	if hi < 0 || ri < 0 {
		t.Fatalf("could not locate cells: header=%q row=%q", hdr, row)
	}
	// Display columns, not bytes: the row prefix contains multibyte runes.
	hCell := ansi.StringWidth(hdr[:hi])
	rCell := ansi.StringWidth(row[:ri])
	if hCell != rCell {
		t.Errorf("header cell at column %d, row cell at %d — misaligned", hCell, rCell)
	}

	// Every cell after the prefix must have identical display width in
	// header and rows, which keeps every " │ " separator on the same
	// column too. Rows have one extra leading part (marker + lineno), so
	// header part i maps to row part i+1.
	hParts := strings.Split(hdr, " │ ")
	rParts := strings.Split(row, " │ ")
	if len(hParts) != len(rParts)-1 {
		t.Fatalf("cell count differs: header %d vs row %d", len(hParts), len(rParts))
	}
	for i := 1; i < len(hParts); i++ {
		if w1, w2 := ansi.StringWidth(hParts[i]), ansi.StringWidth(rParts[i+1]); w1 != w2 {
			t.Errorf("cell %d width %d (header) != %d (row)", i, w1, w2)
		}
	}
}
