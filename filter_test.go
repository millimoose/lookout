package main

import "testing"

func eqEntry(fields map[string]string, raw string) Entry {
	return Entry{Fields: fields, Raw: raw}
}

func TestParseFilterTerms(t *testing.T) {
	f := ParseFilter("level=ERROR payment timeout")
	if len(f.Terms) != 3 {
		t.Fatalf("terms = %d, want 3", len(f.Terms))
	}
	if f.Terms[0].Key != "level" || f.Terms[0].Value != "error" {
		t.Errorf("term0 = %+v", f.Terms[0])
	}
	if f.Terms[1].Key != "" || f.Terms[1].Value != "payment" {
		t.Errorf("term1 = %+v", f.Terms[1])
	}
	if got := f.String(); got != "level=error payment timeout" {
		t.Errorf("String() = %q", got)
	}
}

func TestFilterMatch(t *testing.T) {
	e := eqEntry(map[string]string{"level": "error", "service": "payment-svc"},
		`{"level":"error","service":"payment-svc","msg":"card charge declined"}`)
	malformed := Entry{Raw: "worker crashed unexpectedly"}

	tests := []struct {
		name string
		expr string
		e    Entry
		want bool
	}{
		{"empty matches everything", "", e, true},
		{"key equality", "level=error", e, true},
		{"key equality case-insensitive value", "level=ERROR", e, true},
		{"key equality case-insensitive key", "LEVEL=error", e, true},
		{"key mismatch", "level=warn", e, false},
		{"missing key", "trace_id=abc", e, false},
		{"substring", "declined", e, true},
		{"substring case-insensitive", "DECLINED", e, true},
		{"substring no match", "timeout", e, false},
		{"terms are ANDed", "level=error payment", e, true},
		{"AND failure", "level=error timeout", e, false},
		{"key term never matches malformed", "level=error", malformed, false},
		{"substring matches malformed raw", "crashed", malformed, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseFilter(tt.expr).Match(tt.e); got != tt.want {
				t.Errorf("ParseFilter(%q).Match() = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}
