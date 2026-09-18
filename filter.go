package main

import "strings"

// Term is one filter condition. A term with a Key requires attribute
// equality; a term without one is a substring match against the raw line.
// Both key and value compare case-insensitively.
type Term struct {
	Key   string
	Value string
}

// Filter is the conjunction (AND) of its terms. The zero value matches
// every entry.
type Filter struct {
	Terms []Term
}

// ParseFilter parses a filter expression: whitespace-separated terms, each
// either "key=value" (attribute equality) or a bare substring.
func ParseFilter(expr string) Filter {
	var f Filter
	for _, tok := range strings.Fields(expr) {
		if k, v, ok := strings.Cut(tok, "="); ok {
			f.Terms = append(f.Terms, Term{Key: strings.ToLower(k), Value: strings.ToLower(v)})
		} else {
			f.Terms = append(f.Terms, Term{Value: strings.ToLower(tok)})
		}
	}
	return f
}

func (f Filter) String() string {
	parts := make([]string, len(f.Terms))
	for i, t := range f.Terms {
		if t.Key == "" {
			parts[i] = t.Value
		} else {
			parts[i] = t.Key + "=" + t.Value
		}
	}
	return strings.Join(parts, " ")
}

// Empty reports whether the filter matches everything.
func (f Filter) Empty() bool { return len(f.Terms) == 0 }

// Match reports whether e satisfies every term.
func (f Filter) Match(e Entry) bool {
	for _, t := range f.Terms {
		if !t.Match(e) {
			return false
		}
	}
	return true
}

func (t Term) Match(e Entry) bool {
	if t.Key == "" {
		return strings.Contains(strings.ToLower(e.Raw), t.Value)
	}
	for k, v := range e.Fields {
		if strings.EqualFold(k, t.Key) && strings.EqualFold(v, t.Value) {
			return true
		}
	}
	return false
}
