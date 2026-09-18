package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Entry is one parsed NDJSON line.
type Entry struct {
	LineNo    int               // 1-based source line number
	Raw       string            // original line text
	Fields    map[string]string // attribute name -> stringified value
	Malformed bool              // line was not a JSON object
	ParseErr  string            // why the line is malformed
}

// ParseEntries reads NDJSON from r. It returns the parsed entries (one per
// non-empty line) and lines, the total number of source lines read
// (including empty ones), so tailing can continue numbering from there.
func ParseEntries(r io.Reader) (entries []Entry, lines int, err error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		lines++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		entries = append(entries, ParseLine(lines, raw))
	}
	return entries, lines, sc.Err()
}

// ParseLine parses one NDJSON line into an Entry. A line that is not a JSON
// object yields a malformed entry carrying the error; it never panics.
func ParseLine(lineNo int, raw string) Entry {
	e := Entry{LineNo: lineNo, Raw: raw}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		e.Malformed, e.ParseErr = true, err.Error()
		return e
	}
	if obj == nil { // the line was the literal "null"
		e.Malformed, e.ParseErr = true, "not a JSON object"
		return e
	}
	e.Fields = make(map[string]string, len(obj))
	for k, v := range obj {
		e.Fields[k] = stringify(v)
	}
	return e
}

func stringify(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return "null"
	case map[string]any, []any:
		if b, err := json.Marshal(t); err == nil {
			return string(b)
		}
	}
	return fmt.Sprint(v)
}

// PrettyValue re-indents compact JSON values for the detail view.
func PrettyValue(s string) string {
	if len(s) > 0 && (s[0] == '{' || s[0] == '[') {
		var b bytes.Buffer
		if err := json.Indent(&b, []byte(s), "", "  "); err == nil {
			return b.String()
		}
	}
	return s
}

// priorityKeys are recognized log attributes, rendered as the first columns.
var priorityKeys = []string{
	"timestamp", "time", "ts", "@timestamp",
	"level", "severity", "service", "logger", "msg", "message",
}

// ColumnOrder returns the union of attribute names across entries: known
// log keys first (fixed order), the rest in first-seen order (alphabetical
// within each entry, so the result is deterministic).
func ColumnOrder(entries []Entry) []string {
	seen := make(map[string]bool)
	var order []string
	add := func(k string) {
		if !seen[k] {
			seen[k] = true
			order = append(order, k)
		}
	}
	for _, p := range priorityKeys {
		for _, e := range entries {
			if _, ok := e.Fields[p]; ok {
				add(p)
				break
			}
		}
	}
	for _, e := range entries {
		keys := make([]string, 0, len(e.Fields))
		for k := range e.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			add(k)
		}
	}
	return order
}

// LastLineEnd returns the offset just past the final '\n' in path, so a
// follower can resume without duplicating an already-shown final line that
// lacks a trailing newline. If the file has no newline at all, it returns 0.
func LastLineEnd(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	const window = 64 * 1024
	size := st.Size()
	if size == 0 {
		return 0, nil
	}
	start := size - window
	if start < 0 {
		start = 0
	}
	buf := make([]byte, size-start)
	if _, err := io.ReadFull(io.NewSectionReader(f, start, size-start), buf); err != nil {
		return 0, err
	}
	if idx := bytes.LastIndexByte(buf, '\n'); idx >= 0 {
		return start + int64(idx) + 1, nil
	}
	return 0, nil
}
