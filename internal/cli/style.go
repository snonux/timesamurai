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
	"strconv"
	"strings"
	"time"

	"github.com/snonux/timesamurai/internal/ansi"
	"github.com/snonux/timesamurai/internal/worktime"
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
// which pagers and line-oriented tools handle badly -- and keeps each line's
// surrounding whitespace (indentation, a trailing "\r") outside the
// escapes, so an underline style does not underline it. Callers write whole
// lines (fmt.Fprintf / io.WriteString of "\n"-terminated text at every call
// site), so no cross-call line buffering is needed.
//
// A failed underlying write is reported exactly as the unwrapped writer
// would report it (the legacy package checks these errors), with n = 0
// since the styled bytes written do not map back onto p. On success n is
// len(p): the caller's bytes, not the longer styled rendering.
func (c *lineStyleWriter) Write(p []byte) (int, error) {
	lines := strings.Split(string(p), "\n")
	for i, line := range lines {
		rest := strings.TrimLeft(line, " \t")
		indent := line[:len(line)-len(rest)]
		text := strings.TrimRight(rest, " \t\r")
		lines[i] = indent + c.theme.Wrap(c.pick(line), text) + rest[len(text):]
	}
	if _, err := io.WriteString(c.w, strings.Join(lines, "\n")); err != nil {
		return 0, err
	}
	return len(p), nil
}

// migrateLineRole picks the style for one line of legacy.Migrate's report:
// the summary and succeeded-host lines green, the failed-host breakdown red,
// and everything else (the findings) yellow. The prefixes are legacy's
// wording (writeMigrateReport); a finding can never be mistaken for one of
// them because MigrateFinding.String always starts with "[<kind>]". Should
// legacy reword a line, it degrades to the default yellow rather than
// breaking, and style_test.go's migrate tests pin the real output.
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
// "00:13:20" from ever matching. It is the fallback for a line that is not
// a located row (see colorizeTableAddress).
var tableAddressRe = regexp.MustCompile(`^(\S+:\d+)`)

// tableCellRoles lists the role of each FormatTable column that gets one,
// in column order: ADDRESS, ACTION, WHEN, VALUE, TAGS. DESCR, the last
// column, is free text and deliberately stays plain.
var tableCellRoles = []string{
	ansi.RoleAddress, ansi.RoleKey, ansi.RoleTime, ansi.RoleValue, ansi.RoleTag,
}

// colorizeTable styles a FormatTable(rows) rendering: the header line bold
// and each row's cells by column (address cyan, action, timestamp, value,
// tags; descr stays plain). (tabwriter with flags 0 draws no rule line, so
// there is nothing else to style.) Post-processing after alignment means
// the escape sequences cannot disturb tabwriter's width math.
//
// rows must be the rows table was rendered from. Each row's known cell
// texts are located in its line rather than slicing at fixed column
// offsets, because free text containing one of tabwriter's terminators
// (a newline or form feed in a descr or tag) ends the column block and
// re-aligns every later row to different widths. Rows are matched to lines
// in order: a line that does not start with the next row's cells is a
// continuation of the previous row's free text, not a row.
func colorizeTable(theme ansi.Theme, table string, rows []worktime.Row) string {
	if !theme.Enabled() || table == "" {
		return table
	}
	var out strings.Builder
	next := 0 // index of the row whose line is expected next
	for i, line := range strings.Split(table, "\n") {
		switch {
		case i == 0:
			out.WriteString(theme.Wrap(ansi.RoleHeader, line))
		case line == "":
			out.WriteString(line)
		default:
			styled, isRow := "", false
			if next < len(rows) {
				styled, isRow = colorizeTableRow(theme, line, tableRowCells(rows[next]))
			}
			if isRow {
				next++
			} else {
				styled = colorizeTableAddress(theme, line)
			}
			out.WriteString(styled)
		}
		out.WriteString("\n")
	}
	return strings.TrimSuffix(out.String(), "\n")
}

// tabwriterTerminators are the characters text/tabwriter treats as ending a
// cell ("\t", "\v") or a line ("\n", "\f"), so a field containing one is
// not rendered as one contiguous cell.
const tabwriterTerminators = "\t\v\n\f"

// tableRowCells returns the text of row's styled-column cells as they
// appear on the row's first rendered line, in tableCellRoles order. A field
// containing a tabwriter terminator (a tag with a newline, say) is cut
// there and ends the list: what follows it is rendered in another cell or
// on another line, where it cannot be located reliably, so those cells
// simply stay plain.
func tableRowCells(row worktime.Row) []string {
	fields := []string{
		row.Address,
		row.Entry.Action,
		time.Unix(row.Entry.Epoch, 0).Format(worktime.DisplayTimeLayout),
		strconv.FormatInt(row.Entry.Value, 10),
		strings.Join(row.Entry.Tags, ","),
	}
	for i, field := range fields {
		if cut := strings.IndexAny(field, tabwriterTerminators); cut >= 0 {
			fields[i] = field[:cut]
			return fields[:i+1]
		}
	}
	return fields
}

// colorizeTableRow styles one row line by walking its known cell texts left
// to right: each must appear next, after the padding that follows the
// previous cell. Only the cell text is wrapped; padding and everything
// after the last located cell (the descr) stay outside the escapes. It
// reports false, styling nothing, when a cell is not where it should be --
// the line is then not this row's.
func colorizeTableRow(theme ansi.Theme, line string, cells []string) (string, bool) {
	var out strings.Builder
	rest := line
	for i, cell := range cells {
		if i > 0 {
			text := strings.TrimLeft(rest, " ")
			out.WriteString(rest[:len(rest)-len(text)])
			rest = text
		}
		if !strings.HasPrefix(rest, cell) {
			return "", false
		}
		out.WriteString(theme.Wrap(tableCellRoles[i], cell))
		rest = rest[len(cell):]
	}
	return out.String() + rest, true
}

// colorizeTableAddress is the fallback for a line that is not a located
// row: n9's behavior of styling just a leading "<host>:<id>", which a
// continuation line of free text rarely has.
func colorizeTableAddress(theme ansi.Theme, line string) string {
	return tableAddressRe.ReplaceAllStringFunc(line, func(address string) string {
		return theme.Wrap(ansi.RoleAddress, address)
	})
}
