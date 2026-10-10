// Style helpers (tasks n9 and m83): every colored surface in the CLI
// post-processes the plain formatter output through the runtime's theme
// here, so the domain packages (worktime) never learn about escape sequences
// and a disabled theme collapses each helper to the pre-colors output
// exactly.
//
// n9 styled the highlights (separators, balances, headers, addresses); m83
// extends that to all text output, so the report body and every table column
// are styled as well. The invariant both tasks keep: stripping the escape
// sequences from a styled rendering yields the plain rendering byte for
// byte.
package cli

import (
	"io"
	"regexp"
	"strings"

	"github.com/snonux/timesamurai/internal/ansi"
)

// lineStyleWriter wraps a writer so every line written through it gets the
// style role pick chooses for it. Used for streams produced elsewhere
// (worktime's warning channel, legacy's migrate report and export warning)
// that must not gain an ansi dependency of their own.
type lineStyleWriter struct {
	w     io.Writer
	theme ansi.Theme
	pick  func(line string) string
}

// newColorizeWriter returns an io.Writer that styles each line with role.
// A disabled theme returns w unchanged.
func newColorizeWriter(w io.Writer, theme ansi.Theme, role string) io.Writer {
	return newLineStyleWriter(w, theme, func(string) string { return role })
}

// newLineStyleWriter returns an io.Writer that styles each line with the
// role pick returns for it ("" leaves the line plain). A disabled theme
// returns w unchanged.
func newLineStyleWriter(w io.Writer, theme ansi.Theme, pick func(line string) string) io.Writer {
	if !theme.Enabled() {
		return w
	}
	return &lineStyleWriter{w: w, theme: theme, pick: pick}
}

// Write styles one Write call line by line, so a multi-line write (the
// export discard banner) never has one escape sequence spanning a newline --
// which pagers and line-oriented tools handle badly. Callers write whole
// lines (fmt.Fprintf / io.WriteString of "\n"-terminated text at every call
// site), so no cross-call line buffering is needed.
func (c *lineStyleWriter) Write(p []byte) (int, error) {
	lines := strings.Split(string(p), "\n")
	for i, line := range lines {
		lines[i] = c.theme.Wrap(c.pick(line), line)
	}
	_, _ = io.WriteString(c.w, strings.Join(lines, "\n"))
	return len(p), nil
}

// migrateLineRole picks the style for one line of legacy.Migrate's report:
// the summary and succeeded-host lines green, the failed-host breakdown red,
// and everything else (the findings) yellow.
func migrateLineRole(line string) string {
	switch {
	case strings.HasPrefix(line, "migrated "), strings.HasPrefix(line, "  succeeded:"):
		return ansi.RoleSuccess
	case strings.HasPrefix(line, "  failed:"), strings.HasPrefix(line, "    - "):
		return ansi.RoleError
	default:
		return ansi.RoleWarn
	}
}

// reportSeparatorRe matches the 48-'=' week separator lines FormatReport
// emits between week blocks.
var reportSeparatorRe = regexp.MustCompile(`^=+$`)

// reportDayRe splits a FormatReport day line "   <marker> <Mon YYYYMMDD WW>:"
// into indent+marker, the day label, and the rest (the category tokens).
var reportDayRe = regexp.MustCompile(`^(   )([* ])( )([A-Z][a-z]{2} \d{8} \d{2})(:)(.*)$`)

// reportTokenRe matches one " key:1.23h" category token (day and week
// lines), including its leading space so a balance token keeps n9's
// whole-token styling, sign and all.
var reportTokenRe = regexp.MustCompile(` ([a-z]+):(-?\d+\.\d{2}h)`)

// reportEpochRe matches the --verbose day line's trailing
// " epoch:<n>(<time>)" annotation.
var reportEpochRe = regexp.MustCompile(` epoch:\d+\(.*\)$`)

// colorizeReport styles a FormatReport rendering: separator lines faint,
// day labels as timestamps, category tokens as key/value pairs, and week
// balance tokens green when >= 0 and red when negative. Line-scoped
// post-processing (never pre-formatting) keeps tabwriter-free report
// alignment intact and leaves the worktime formatter byte-identical for the
// golden tests.
func colorizeReport(theme ansi.Theme, report string) string {
	if !theme.Enabled() || report == "" {
		return report
	}
	var out strings.Builder
	for _, line := range strings.Split(report, "\n") {
		switch {
		case reportSeparatorRe.MatchString(line) && line != "":
			out.WriteString(theme.Wrap(ansi.RoleSeparator, line))
		default:
			out.WriteString(colorizeReportLine(theme, line))
		}
		out.WriteString("\n")
	}
	// Splitting an input that ends in "\n" yields a phantom final empty
	// element, so the builder accumulated one "\n" too many; trim it back
	// off to keep the colorized output byte-aligned with the plain one.
	return strings.TrimSuffix(out.String(), "\n")
}

// colorizeReportLine styles one non-separator report line: a day line gets
// its marker and label styled before its tokens, a week totals line is
// tokens only. A line matching neither shape passes through unchanged.
func colorizeReportLine(theme ansi.Theme, line string) string {
	day := reportDayRe.FindStringSubmatch(line)
	if day == nil {
		return colorizeReportTokens(theme, line)
	}
	// day[2] is "*" (day off / weekend) or " "; Wrap of the latter would
	// only add invisible escapes, so only the real marker is styled.
	marker := day[2]
	if marker == "*" {
		marker = theme.Wrap(ansi.RoleWarn, marker)
	}
	return day[1] + marker + day[3] + theme.Wrap(ansi.RoleTime, day[4]) + day[5] +
		colorizeReportTokens(theme, day[6])
}

// colorizeReportTokens styles every " key:1.23h" token in text plus the
// verbose epoch annotation, which is faint since it is debugging detail.
func colorizeReportTokens(theme ansi.Theme, text string) string {
	epoch := reportEpochRe.FindString(text)
	text = strings.TrimSuffix(text, epoch)
	styled := reportTokenRe.ReplaceAllStringFunc(text, func(token string) string {
		return colorizeReportToken(theme, token)
	})
	return styled + theme.Wrap(ansi.RoleDim, epoch)
}

// colorizeReportToken styles one " key:1.23h" token: the balance token as a
// whole (positive green, negative red, leading space included, exactly as
// n9 shipped it), any other category as a key/value pair.
func colorizeReportToken(theme ansi.Theme, token string) string {
	match := reportTokenRe.FindStringSubmatch(token)
	key, value := match[1], match[2]
	if key != "balance" {
		return " " + theme.Wrap(ansi.RoleKey, key) + ":" + theme.Wrap(ansi.RoleValue, value)
	}
	role := ansi.RolePositive
	if strings.HasPrefix(value, "-") {
		role = ansi.RoleNegative
	}
	return theme.Wrap(role, token)
}

// tableAddressRe matches a table row's leading "<host>:<id>" cell so only
// the address column is styled, never the descr free-text. After tabwriter
// alignment the row's cells are space-separated, so the anchor (line start)
// plus the mandatory ":<digits>" tail is what keeps the timestamp column's
// "00:13:20" from ever matching. It is the fallback for a row that does not
// line up with the header's columns (see colorizeTableRow).
var tableAddressRe = regexp.MustCompile(`^(\S+:\d+)`)

// tableColumns lists FormatTable's header names with the role each column's
// cells get. DESCR is free text and deliberately has none.
var tableColumns = []struct{ name, role string }{
	{"ADDRESS", ansi.RoleAddress},
	{"ACTION", ansi.RoleKey},
	{"WHEN", ansi.RoleTime},
	{"VALUE", ansi.RoleValue},
	{"TAGS", ansi.RoleTag},
	{"DESCR", ""},
}

// colorizeTable styles a FormatTable rendering: the header line bold and
// each row's cells by column (address cyan, action, timestamp, value, tags;
// descr stays plain). (tabwriter with flags 0 draws no rule line, so there
// is nothing else to style.) Post-processing after alignment means the
// escape sequences cannot disturb tabwriter's width math.
func colorizeTable(theme ansi.Theme, table string) string {
	if !theme.Enabled() || table == "" {
		return table
	}
	lines := strings.Split(table, "\n")
	offsets := tableColumnOffsets(lines[0])
	var out strings.Builder
	for i, line := range lines {
		switch {
		case i == 0:
			out.WriteString(theme.Wrap(ansi.RoleHeader, line))
		case line != "":
			out.WriteString(colorizeTableRow(theme, line, offsets))
		default:
			out.WriteString(line)
		}
		out.WriteString("\n")
	}
	return strings.TrimSuffix(out.String(), "\n")
}

// tableColumnOffsets returns the rune offset at which each tableColumns
// entry starts in header, or nil when header is not FormatTable's header.
// Offsets are in runes because that is what tabwriter aligns on (the header
// itself is ASCII, so its byte and rune offsets coincide).
func tableColumnOffsets(header string) []int {
	offsets := make([]int, len(tableColumns))
	from := 0
	for i, column := range tableColumns {
		at := strings.Index(header[from:], column.name)
		if at < 0 {
			return nil
		}
		offsets[i] = from + at
		from = offsets[i] + len(column.name)
	}
	return offsets
}

// colorizeTableRow styles one aligned row cell by cell using the header's
// column offsets. Only each cell's text is wrapped; the padding between
// columns stays outside the escapes. A row that does not line up with the
// header (a descr containing a newline or tab breaks tabwriter's single
// column block) falls back to styling just its address, n9's behavior,
// rather than guessing where its cells are.
func colorizeTableRow(theme ansi.Theme, line string, offsets []int) string {
	row := []rune(line)
	if !rowAligned(row, offsets) {
		return tableAddressRe.ReplaceAllStringFunc(line, func(address string) string {
			return theme.Wrap(ansi.RoleAddress, address)
		})
	}
	var out strings.Builder
	for i, column := range tableColumns {
		end := len(row)
		if i+1 < len(offsets) {
			end = offsets[i+1]
		}
		cell := string(row[offsets[i]:end])
		text := strings.TrimRight(cell, " ")
		if i+1 == len(offsets) {
			// DESCR is the last column: unpadded, and trailing spaces in
			// it are the user's own text.
			text = cell
		}
		out.WriteString(theme.Wrap(column.role, text))
		out.WriteString(cell[len(text):])
	}
	return out.String()
}

// rowAligned reports whether row has a cell boundary at every column
// offset: long enough to reach the last column, and with tabwriter's
// two-space padding immediately before each column start.
func rowAligned(row []rune, offsets []int) bool {
	if len(offsets) == 0 || len(row) < offsets[len(offsets)-1] {
		return false
	}
	for _, offset := range offsets[1:] {
		if offset < 2 || row[offset-1] != ' ' || row[offset-2] != ' ' {
			return false
		}
	}
	return true
}
