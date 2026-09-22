// Style helpers for task n9: every colored surface in the CLI post-processes
// the plain formatter output through the runtime's theme here, so the
// domain packages (worktime) never learn about escape sequences and a
// disabled theme collapses each helper to the pre-colors output exactly.
package cli

import (
	"io"
	"regexp"
	"strings"

	"github.com/snonux/timesamurai/internal/ansi"
)

// colorizeWriter wraps a writer so every line written through it gets
// role's style. Used for streams produced elsewhere (worktime's warning
// channel) that must not gain an ansi dependency of their own.
type colorizeWriter struct {
	w     io.Writer
	theme ansi.Theme
	role  string
}

// newColorizeWriter returns an io.Writer that styles each line with role.
// A disabled theme returns w unchanged.
func newColorizeWriter(w io.Writer, theme ansi.Theme, role string) io.Writer {
	if !theme.Enabled() {
		return w
	}
	return &colorizeWriter{w: w, theme: theme, role: role}
}

// Write styles one full Write call (callers write whole lines, matching
// fmt.Fprintf usage at every warn-writer call site).
func (c *colorizeWriter) Write(p []byte) (int, error) {
	_, _ = io.WriteString(c.w, c.theme.Wrap(c.role, string(p)))
	return len(p), nil
}

// reportSeparatorRe matches the 48-'=' week separator lines FormatReport
// emits between week blocks.
var reportSeparatorRe = regexp.MustCompile(`^=+$`)

// reportBalanceRe matches the week totals line's balance token, including
// its leading space, so a negative value colors the sign too.
var reportBalanceRe = regexp.MustCompile(` balance:(-?\d+\.\d{2})h`)

// colorizeReport styles a FormatReport rendering: separator lines faint,
// week balance tokens green when >= 0 and red when negative. Line-scoped
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
			out.WriteString(colorizeBalance(theme, line))
		}
		out.WriteString("\n")
	}
	// Splitting an input that ends in "\n" yields a phantom final empty
	// element, so the builder accumulated one "\n" too many; trim it back
	// off to keep the colorized output byte-aligned with the plain one.
	return strings.TrimSuffix(out.String(), "\n")
}

// colorizeBalance styles one line's balance token (positive green, negative
// red) when the line carries one. Lines without a balance token pass
// through unchanged.
func colorizeBalance(theme ansi.Theme, line string) string {
	match := reportBalanceRe.FindStringSubmatchIndex(line)
	if match == nil {
		return line
	}
	tokenStart, tokenEnd := match[0], match[1]
	value := line[match[2]:match[3]]
	role := ansi.RolePositive
	if strings.HasPrefix(value, "-") {
		role = ansi.RoleNegative
	}
	return line[:tokenStart] + theme.Wrap(role, line[tokenStart:tokenEnd]) + line[tokenEnd:]
}

// tableAddressRe matches a table row's leading "<host>:<id>" cell so only
// the address column is styled, never the descr free-text. After tabwriter
// alignment the row's cells are space-separated, so the anchor (line start)
// plus the mandatory ":<digits>" tail is what keeps the timestamp column's
// "00:13:20" from ever matching.
var tableAddressRe = regexp.MustCompile(`^(\S+:\d+)`)

// colorizeTable styles a FormatTable rendering: the header line bold and
// each row's address cell cyan. (tabwriter with flags 0 draws no rule line,
// so there is nothing else to style.) Post-processing after alignment means
// the escape sequences cannot disturb tabwriter's width math.
func colorizeTable(theme ansi.Theme, table string) string {
	if !theme.Enabled() || table == "" {
		return table
	}
	var out strings.Builder
	for i, line := range strings.Split(table, "\n") {
		switch {
		case i == 0:
			out.WriteString(theme.Wrap(ansi.RoleHeader, line))
		case line != "":
			out.WriteString(tableAddressRe.ReplaceAllStringFunc(line, func(address string) string {
				return theme.Wrap(ansi.RoleAddress, address)
			}))
		default:
			out.WriteString(line)
		}
		out.WriteString("\n")
	}
	return strings.TrimSuffix(out.String(), "\n")
}
