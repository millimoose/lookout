package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	keybindsTable  = "↑/↓ scroll · / filter · c columns · f follow · enter detail · q quit"
	keybindsFilter = "enter apply · esc cancel   (key=value matches an attribute, bare words match text; space = AND)"
	keybindsPick   = "↑/↓ move · space toggle · a all/none · esc close"
	keybindsDetail = "↑/↓ scroll · esc back"

	maxColWidth = 40
)

var (
	styHeader    = lipgloss.NewStyle().Bold(true)
	styFaint     = lipgloss.NewStyle().Faint(true)
	stySel       = lipgloss.NewStyle().Bold(true)
	styErr       = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	styWarn      = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styPickerSel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
)

type mode int

const (
	modeTable mode = iota
	modeFilter
	modeColumns
	modeDetail
)

type linesMsg FollowEvent

func waitLines(ch <-chan FollowEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return linesMsg(ev)
	}
}

// Model is the bubbletea model for the whole viewer.
type Model struct {
	path     string
	entries  []Entry
	visible  []int // indices into entries matching the filter
	nextLine int   // next source line number to assign to appended lines

	colOrder []string
	colSeen  map[string]bool
	hidden   map[string]bool
	widths   map[string]int

	filter      Filter
	filterExpr  string
	filterInput textinput.Model

	followCh <-chan FollowEvent
	follow   bool // stick to the bottom as new lines arrive

	mode        mode
	cursor      int // index into visible
	scrollOff   int // first displayed row
	pickerCur   int
	pickerOff   int
	detailOff   int
	truncateHit bool

	width, height int
	malformed     int
}

func NewModel(path string, entries []Entry, lines int, followCh <-chan FollowEvent, follow bool) Model {
	ti := textinput.New()
	ti.Placeholder = "level=error payment"
	ti.Prompt = "filter: "
	m := Model{
		path:        path,
		entries:     entries,
		nextLine:    lines,
		colOrder:    ColumnOrder(entries),
		colSeen:     make(map[string]bool),
		hidden:      make(map[string]bool),
		widths:      make(map[string]int),
		filterInput: ti,
		followCh:    followCh,
		follow:      follow,
		mode:        modeTable,
		width:       80,
		height:      24,
	}
	for _, c := range m.colOrder {
		m.colSeen[c] = true
		m.widths[c] = ansi.StringWidth(c)
	}
	for _, e := range entries {
		m.absorb(e)
	}
	m.rebuild()
	if follow && len(m.visible) > 0 {
		m.cursor = len(m.visible) - 1
		m.clampScroll()
	}
	return m
}

func (m Model) Init() tea.Cmd {
	if m.followCh != nil {
		return waitLines(m.followCh)
	}
	return nil
}

// absorb accounts for one entry's contribution to column set, widths, stats.
func (m *Model) absorb(e Entry) {
	if e.Malformed {
		m.malformed++
	}
	for k, v := range e.Fields {
		if !m.colSeen[k] {
			m.colSeen[k] = true
			m.colOrder = append(m.colOrder, k)
			m.widths[k] = ansi.StringWidth(k)
		}
		if w := ansi.StringWidth(v); w > m.widths[k] {
			m.widths[k] = w
		}
	}
}

func (m *Model) rebuild() {
	m.visible = m.visible[:0]
	for i := range m.entries {
		if m.filter.Match(m.entries[i]) {
			m.visible = append(m.visible, i)
		}
	}
	if len(m.visible) == 0 {
		m.cursor = 0
	} else if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	m.clampScroll()
}

func (m *Model) appendLines(lines []string) {
	for _, ln := range lines {
		m.nextLine++
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" {
			continue
		}
		e := ParseLine(m.nextLine, trimmed)
		m.entries = append(m.entries, e)
		m.absorb(e)
	}
	m.rebuild()
	if m.follow && len(m.visible) > 0 {
		m.cursor = len(m.visible) - 1
		m.clampScroll()
	}
}

func (m *Model) rowsAvail() int {
	if n := m.height - 3; n > 0 {
		return n
	}
	return 1
}

func (m *Model) clampScroll() {
	if m.cursor < m.scrollOff {
		m.scrollOff = m.cursor
	}
	if m.cursor >= m.scrollOff+m.rowsAvail() {
		m.scrollOff = m.cursor - m.rowsAvail() + 1
	}
	if m.scrollOff < 0 {
		m.scrollOff = 0
	}
}

func (m *Model) move(d int) {
	if len(m.visible) == 0 {
		return
	}
	m.cursor += d
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.visible)-1 {
		m.cursor = len(m.visible) - 1
	}
	m.clampScroll()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if w := msg.Width - 16; w > 10 {
			m.filterInput.Width = w
		}
		m.clampScroll()
		return m, nil

	case linesMsg:
		m.truncateHit = msg.Truncated
		m.appendLines(msg.Lines)
		return m, waitLines(m.followCh)

	case tea.KeyMsg:
		switch m.mode {
		case modeFilter:
			switch msg.String() {
			case "enter":
				m.filterExpr = m.filterInput.Value()
				m.filter = ParseFilter(m.filterExpr)
				m.cursor = 0
				m.rebuild()
				if m.follow && len(m.visible) > 0 {
					m.cursor = len(m.visible) - 1
				}
				m.mode = modeTable
			case "esc":
				m.filterInput.SetValue(m.filterExpr)
				m.mode = modeTable
			default:
				var cmd tea.Cmd
				m.filterInput, cmd = m.filterInput.Update(msg)
				return m, cmd
			}

		case modeColumns:
			switch msg.String() {
			case "esc", "enter":
				m.mode = modeTable
			case "up", "k":
				if m.pickerCur > 0 {
					m.pickerCur--
				}
			case "down", "j":
				if m.pickerCur < len(m.colOrder)-1 {
					m.pickerCur++
				}
			case " ", "space":
				if len(m.colOrder) > 0 {
					col := m.colOrder[m.pickerCur]
					if m.hidden[col] {
						delete(m.hidden, col)
					} else {
						m.hidden[col] = true
					}
				}
			case "a":
				if len(m.hidden) == len(m.colOrder) {
					m.hidden = make(map[string]bool)
				} else {
					m.hidden = make(map[string]bool)
					for _, c := range m.colOrder {
						m.hidden[c] = true
					}
				}
			}
			m.clampPicker()

		case modeDetail:
			switch msg.String() {
			case "esc", "q", "enter":
				m.mode = modeTable
			case "up", "k":
				if m.detailOff > 0 {
					m.detailOff--
				}
			case "down", "j":
				if m.detailOff < m.maxDetailOff() {
					m.detailOff++
				}
			}

		default: // modeTable
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "up", "k":
				m.move(-1)
			case "down", "j":
				m.move(1)
			case "pgup":
				m.move(-m.rowsAvail())
			case "pgdown":
				m.move(m.rowsAvail())
			case "g", "home":
				m.cursor = 0
				m.clampScroll()
			case "G", "end":
				if len(m.visible) > 0 {
					m.cursor = len(m.visible) - 1
					m.clampScroll()
				}
			case "/":
				m.mode = modeFilter
				m.filterInput.Focus()
				return m, textinput.Blink
			case "f":
				m.follow = !m.follow
				if m.follow && len(m.visible) > 0 {
					m.cursor = len(m.visible) - 1
					m.clampScroll()
				}
			case "c":
				m.mode = modeColumns
				m.pickerCur, m.pickerOff = 0, 0
				m.clampPicker()
			case "enter":
				if len(m.visible) > 0 {
					m.mode = modeDetail
					m.detailOff = 0
				}
			}
		}
	}
	return m, nil
}

func (m *Model) clampPicker() {
	h := m.pickerHeight()
	if m.pickerCur < m.pickerOff {
		m.pickerOff = m.pickerCur
	}
	if m.pickerCur >= m.pickerOff+h {
		m.pickerOff = m.pickerCur - h + 1
	}
	if m.pickerOff < 0 {
		m.pickerOff = 0
	}
}

func (m Model) pickerHeight() int {
	if h := m.height - 2; h > 1 {
		return h
	}
	return 1
}

func (m Model) maxDetailOff() int {
	n := len(m.detailLines()) - (m.height - 1)
	if n < 0 {
		return 0
	}
	return n
}

func (m Model) entryAtCursor() Entry {
	return m.entries[m.visible[m.cursor]]
}

// ---- rendering ----

func (m Model) visibleCols() []string {
	cols := make([]string, 0, len(m.colOrder))
	for _, c := range m.colOrder {
		if !m.hidden[c] {
			cols = append(cols, c)
		}
	}
	return cols
}

func (m Model) levelCol() string {
	for _, k := range []string{"level", "severity"} {
		if m.colSeen[k] {
			return k
		}
	}
	return ""
}

func fit(s string, w int) string {
	if w < 1 {
		return ""
	}
	t := ansi.Truncate(s, w, "…")
	if pad := w - ansi.StringWidth(t); pad > 0 {
		return t + strings.Repeat(" ", pad)
	}
	return t
}

func styleForLevel(col, val string) lipgloss.Style {
	if col == "" {
		return lipgloss.NewStyle()
	}
	switch strings.ToLower(val) {
	case "error", "err", "fatal", "panic", "critical", "dpanic":
		return styErr
	case "warn", "warning":
		return styWarn
	default:
		return lipgloss.NewStyle()
	}
}

func (m Model) lineNoWidth() int {
	w := len(fmt.Sprint(m.nextLine))
	if w < 4 {
		w = 4
	}
	return w
}

func (m Model) headerLine() string {
	lw := m.lineNoWidth()
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", lw+4))
	cells := []string{}
	for _, c := range m.visibleCols() {
		w := m.widths[c]
		if w > maxColWidth {
			w = maxColWidth
		}
		cells = append(cells, fit(c, w))
	}
	b.WriteString(styFaint.Render(strings.Join(cells, styFaint.Render(" │ "))))
	return ansi.Truncate(b.String(), m.width, "…")
}

func (m Model) rowLine(visIdx int) string {
	e := m.entries[m.visible[visIdx]]
	lw := m.lineNoWidth()
	marker := " "
	if visIdx == m.cursor {
		marker = stySel.Render("❯")
	}
	lineno := styFaint.Render(fmt.Sprintf("%*d", lw, e.LineNo))
	sep := styFaint.Render(" │ ")

	if e.Malformed {
		rest := m.width - lw - 4
		msg := "✗ invalid JSON — " + e.ParseErr + " · " + e.Raw
		return marker + " " + lineno + sep + styErr.Render(fit(msg, rest))
	}

	var cells []string
	for _, c := range m.visibleCols() {
		w := m.widths[c]
		if w > maxColWidth {
			w = maxColWidth
		}
		cells = append(cells, fit(styleForLevel(m.levelCol(), e.Fields[c]).Render(e.Fields[c]), w))
	}
	line := marker + " " + lineno + sep + strings.Join(cells, sep)
	return ansi.Truncate(line, m.width, "…")
}

func (m Model) statusLine() string {
	var parts []string
	parts = append(parts, styHeader.Render(m.path))
	parts = append(parts, fmt.Sprintf("%d entries · %d shown", len(m.entries), len(m.visible)))
	if m.malformed > 0 {
		parts = append(parts, styErr.Render(fmt.Sprintf("%d malformed", m.malformed)))
	}
	if m.filterExpr != "" {
		parts = append(parts, "filter: "+stySel.Render(m.filterExpr))
	}
	if m.follow {
		parts = append(parts, stySel.Reverse(true).Render(" FOLLOW "))
	}
	if m.truncateHit {
		parts = append(parts, styFaint.Render("file truncated — re-reading from start"))
	}
	return strings.Join(parts, " · ")
}

func (m Model) tableView() string {
	var b strings.Builder
	b.WriteString(m.headerLine())
	b.WriteByte('\n')
	for i := range m.rowsAvail() {
		idx := m.scrollOff + i
		if idx >= len(m.visible) {
			break
		}
		b.WriteString(m.rowLine(idx))
		b.WriteByte('\n')
	}
	b.WriteString(ansi.Truncate(m.statusLine(), m.width, "…"))
	b.WriteByte('\n')
	switch m.mode {
	case modeFilter:
		b.WriteString(m.filterInput.View())
	default:
		b.WriteString(styFaint.Render(keybindsTable))
	}
	return b.String()
}

func (m Model) pickerView() string {
	var b strings.Builder
	b.WriteString(styHeader.Render("Columns") + styFaint.Render("  (space toggles, hidden columns stay in the detail view)"))
	b.WriteByte('\n')
	h := m.pickerHeight()
	for i := range h {
		idx := m.pickerOff + i
		if idx >= len(m.colOrder) {
			break
		}
		col := m.colOrder[idx]
		box := "[ ] "
		if !m.hidden[col] {
			box = "[x] "
		}
		line := "  " + box + col
		if idx == m.pickerCur {
			line = "❯ " + box + col
			b.WriteString(styPickerSel.Render(ansi.Truncate(line, m.width, "…")))
		} else {
			b.WriteString(ansi.Truncate(line, m.width, "…"))
		}
		b.WriteByte('\n')
	}
	b.WriteString(styFaint.Render(keybindsPick))
	return b.String()
}

// detailLines builds the detail view for the entry at the cursor.
func (m Model) detailLines() []string {
	if len(m.visible) == 0 {
		return nil
	}
	e := m.entryAtCursor()
	var lines []string
	if e.Malformed {
		lines = append(lines,
			styErr.Render(fmt.Sprintf("✗ line %d — malformed JSON", e.LineNo)),
			"",
			styFaint.Render("parse error:"), e.ParseErr,
			"", styFaint.Render("raw line:"), e.Raw)
	} else {
		lines = append(lines, fmt.Sprintf("line %d · %d attributes", e.LineNo, len(e.Fields)), "")
		order := make([]string, 0, len(e.Fields))
		order = append(order, m.colOrder...)
		inCols := make(map[string]bool, len(m.colOrder))
		for _, c := range m.colOrder {
			inCols[c] = true
		}
		var rest []string
		for k := range e.Fields {
			if !inCols[k] {
				rest = append(rest, k)
			}
		}
		sort.Strings(rest)
		order = append(order, rest...)
		for _, k := range order {
			if v, ok := e.Fields[k]; ok {
				lines = append(lines, styHeader.Render(k)+": "+PrettyValue(v))
			}
		}
	}
	return lines
}

func (m Model) detailView() string {
	avail := m.height - 1
	if avail < 1 {
		avail = 1
	}
	w := m.width - 2
	if w < 10 {
		w = 10
	}
	var phys []string
	for _, l := range m.detailLines() {
		if ansi.StringWidth(l) <= w {
			phys = append(phys, l)
		} else {
			phys = append(phys, strings.Split(lipgloss.NewStyle().Width(w).Render(l), "\n")...)
		}
	}
	off := m.detailOff
	if off > len(phys) {
		off = len(phys)
	}
	end := off + avail
	if end > len(phys) {
		end = len(phys)
	}
	var b strings.Builder
	for _, l := range phys[off:end] {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	b.WriteString(styFaint.Render(keybindsDetail))
	return b.String()
}

func (m Model) View() string {
	switch m.mode {
	case modeColumns:
		return m.pickerView()
	case modeDetail:
		return m.detailView()
	default:
		return m.tableView()
	}
}
