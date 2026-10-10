package cli

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/snonux/timesamurai/internal/ansi"
	"github.com/snonux/timesamurai/internal/worktime"
)

// stripSGR removes all ANSI escape sequences, so tests can assert that a
// colorized rendering still carries exactly the same visible text as the
// plain one.
var stripSGR = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func enabledTheme(t *testing.T) ansi.Theme {
	t.Helper()
	theme, err := ansi.NewTheme(true, nil)
	if err != nil {
		t.Fatalf("NewTheme: %v", err)
	}
	return theme
}

func disabledTheme(t *testing.T) ansi.Theme {
	t.Helper()
	theme, err := ansi.NewTheme(false, nil)
	if err != nil {
		t.Fatalf("NewTheme: %v", err)
	}
	return theme
}

func TestColorizeReportDisabledIsIdentity(t *testing.T) {
	plain := sampleReport(t, disabledTheme(t))
	if strings.ContainsRune(plain, '\x1b') {
		t.Fatalf("disabled theme emitted escape sequences: %q", plain)
	}
}

func TestColorizeReportStylesSeparatorAndBalance(t *testing.T) {
	theme := enabledTheme(t)
	plain := sampleReport(t, disabledTheme(t))
	styled := colorizeReport(theme, plain)

	if !strings.Contains(styled, "\x1b[2m====") {
		t.Errorf("separator line not faint-styled:\n%q", styled)
	}
	if !strings.Contains(styled, "\x1b[31m balance:-33.00h\x1b[0m") {
		t.Errorf("negative balance not red:\n%q", styled)
	}
	if !strings.Contains(styled, "\x1b[32m balance:7.00h\x1b[0m") {
		t.Errorf("positive balance not green:\n%q", styled)
	}
	// Visible text must be byte-identical once escapes are stripped.
	if got := stripSGR.ReplaceAllString(styled, ""); got != plain {
		t.Errorf("colorized visible text differs from plain:\n got %q\nwant %q", got, plain)
	}
}

// sampleReport builds a two-week report through the real formatter. The
// balance is the running cross-week total (7h then 80h of work against a
// 40h weekly target gives -33.00h then +7.00h cumulative), exercising both
// balance color roles on real FormatReport output shapes.
func sampleReport(t *testing.T, theme ansi.Theme) string {
	t.Helper()
	cfg := worktime.AccountingConfig{
		WeekWorkHours: 40,
		PlusFor:       []string{"off", "bank", "bufferuse", "sick"},
		MinusFor:      []string{"lunch"},
		WeekendDays:   []string{"Sat", "Sun"},
	}
	entries := []worktime.Entry{
		{Host: "h1", ID: 1, Action: "add", Epoch: time.Date(2024, 1, 2, 9, 0, 0, 0, time.UTC).Unix(), Value: 7 * 3600, Tags: []string{"work"}},
		{Host: "h1", ID: 2, Action: "add", Epoch: time.Date(2024, 1, 9, 9, 0, 0, 0, time.UTC).Unix(), Value: 80 * 3600, Tags: []string{"work"}},
	}
	weeks, err := worktime.BuildReport(entries, cfg, nil)
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	return colorizeReport(theme, worktime.FormatReport(weeks, false))
}

func TestColorizeTableStylesHeaderAndAddresses(t *testing.T) {
	theme := enabledTheme(t)
	rows := []worktime.Row{
		{Address: "host1:1", Entry: worktime.Entry{Host: "host1", ID: 1, Action: "add", Epoch: 1700000000, Value: 3600, Tags: []string{"work"}}},
	}
	plain := worktime.FormatTable(rows)
	styled := colorizeTable(theme, plain)

	if !strings.HasPrefix(styled, "\x1b[1mADDRESS") {
		t.Errorf("header line not bold:\n%q", styled)
	}
	if !strings.Contains(styled, "\x1b[36mhost1:1\x1b[0m") {
		t.Errorf("address cell not cyan:\n%q", styled)
	}
	if got := stripSGR.ReplaceAllString(styled, ""); got != plain {
		t.Errorf("colorized visible text differs from plain:\n got %q\nwant %q", got, plain)
	}
}

func TestColorizeTableDisabledIsIdentity(t *testing.T) {
	rows := []worktime.Row{
		{Address: "host1:1", Entry: worktime.Entry{Host: "host1", ID: 1, Action: "add", Epoch: 1700000000}},
	}
	plain := worktime.FormatTable(rows)
	if got := colorizeTable(disabledTheme(t), plain); got != plain {
		t.Errorf("disabled theme changed table output:\n got %q\nwant %q", got, plain)
	}
}

// TestReportOutputColoredWhenForced runs the real `work report` command with
// CLICOLOR_FORCE=1 (colors on despite the test's non-TTY stdout) and asserts
// escape sequences reach the output.
func TestReportOutputColoredWhenForced(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "--at", "2024-01-02T09:00:00Z"); err != nil {
		t.Fatalf("seed add: %v", err)
	}

	t.Setenv("CLICOLOR_FORCE", "1")
	forced, err := runWork(t, store, "report", "2024-01-01..2024-01-31")
	if err != nil {
		t.Fatalf("report (forced): %v", err)
	}
	if !strings.Contains(forced, "\x1b[") {
		t.Errorf("forced-color report has no escape sequences:\n%q", forced)
	}
}

// TestReportOutputPlainWithoutForce guards the inverse: without
// CLICOLOR_FORCE (and without a tty), report output must stay byte-plain.
func TestReportOutputPlainWithoutForce(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "--at", "2024-01-02T09:00:00Z"); err != nil {
		t.Fatalf("seed add: %v", err)
	}
	out, err := runWork(t, store, "report", "2024-01-01..2024-01-31")
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if strings.ContainsRune(out, '\x1b') {
		t.Errorf("plain report leaked escape sequences:\n%q", out)
	}
}

// TestNoColorFlagBeatsForce pins --no-color's position in the precedence:
// even with CLICOLOR_FORCE=1, an explicit --no-color disables all styling.
func TestNoColorFlagBeatsForce(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "--at", "2024-01-02T09:00:00Z"); err != nil {
		t.Fatalf("seed add: %v", err)
	}
	t.Setenv("CLICOLOR_FORCE", "1")
	out, err := runWork(t, store, "report", "2024-01-01..2024-01-31", "--no-color")
	if err != nil {
		t.Fatalf("report --no-color: %v", err)
	}
	if strings.ContainsRune(out, '\x1b') {
		t.Errorf("--no-color still emitted escape sequences:\n%q", out)
	}
}

// TestColorizeReportEmptyStaysEmpty pins the guard against the empty-range
// report gaining stray newlines when colors are enabled (review finding).
func TestColorizeReportEmptyStaysEmpty(t *testing.T) {
	theme := enabledTheme(t)
	if got := colorizeReport(theme, ""); got != "" {
		t.Errorf("colorizeReport(\"\") = %q, want empty", got)
	}
	if got := colorizeTable(theme, ""); got != "" {
		t.Errorf("colorizeTable(\"\") = %q, want empty", got)
	}
}

// TestColorizeReportStylesDayLinesAndTokens pins task m83's report body
// styling on real FormatReport output: the day label as a timestamp, each
// category token as a key/value pair, and the week line's non-balance
// tokens too.
func TestColorizeReportStylesDayLinesAndTokens(t *testing.T) {
	styled := sampleReport(t, enabledTheme(t))

	for _, want := range []string{
		"\x1b[34mTue 20240102 01\x1b[0m:", // day label
		" \x1b[35mwork\x1b[0m:\x1b[1m7.00h\x1b[0m",
		" \x1b[35mbuffer\x1b[0m:\x1b[1m0.00h\x1b[0m", // week totals line
	} {
		if !strings.Contains(styled, want) {
			t.Errorf("styled report missing %q:\n%q", want, styled)
		}
	}
	// The balance token keeps its whole-token style and must not also be
	// split into a key/value pair.
	if strings.Contains(styled, "\x1b[35mbalance") {
		t.Errorf("balance token styled as a plain key:\n%q", styled)
	}
}

// TestColorizeReportLineShapes covers the day line variants FormatReport can
// emit that sampleReport does not: the '*' marker and the --verbose epoch
// annotation. Every case must also strip back to its exact input.
func TestColorizeReportLineShapes(t *testing.T) {
	theme := enabledTheme(t)
	cases := []struct{ name, line, want string }{
		{
			name: "weekend marker",
			line: "   * Sat 20240106 01: work:1.50h",
			want: "   \x1b[33m*\x1b[0m \x1b[34mSat 20240106 01\x1b[0m: \x1b[35mwork\x1b[0m:\x1b[1m1.50h\x1b[0m",
		},
		{
			name: "verbose epoch annotation is faint, not a token",
			line: "     Tue 20240102 01: work:7.00h epoch:1704186000(2024-01-02 09:00:00 +0000 UTC)",
			want: "     \x1b[34mTue 20240102 01\x1b[0m: \x1b[35mwork\x1b[0m:\x1b[1m7.00h\x1b[0m" +
				"\x1b[2m epoch:1704186000(2024-01-02 09:00:00 +0000 UTC)\x1b[0m",
		},
		{
			name: "negative non-balance value keeps its sign in the value",
			line: "     Tue 20240102 01: work:-1.00h lunch:0.50h",
			want: "     \x1b[34mTue 20240102 01\x1b[0m: \x1b[35mwork\x1b[0m:\x1b[1m-1.00h\x1b[0m \x1b[35mlunch\x1b[0m:\x1b[1m0.50h\x1b[0m",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := colorizeReportLine(theme, tc.line)
			if got != tc.want {
				t.Errorf("colorizeReportLine:\n got %q\nwant %q", got, tc.want)
			}
			if stripped := stripSGR.ReplaceAllString(got, ""); stripped != tc.line {
				t.Errorf("visible text changed:\n got %q\nwant %q", stripped, tc.line)
			}
		})
	}
}

// TestColorizeReportLineLeavesUnknownShapesPlain is the negative case: text
// that merely resembles report output (no hours suffix, uppercase key, a
// malformed day label) must pass through without a single escape.
func TestColorizeReportLineLeavesUnknownShapesPlain(t *testing.T) {
	theme := enabledTheme(t)
	for _, line := range []string{
		"",
		"some free text",
		" work:7.00",                  // no "h" suffix
		" WORK:7.00h",                 // keys are lowercase
		" work:7h",                    // not two decimals
		"   * Sat 2024-01-06 01:",     // label is not the compact date
		"warning: skipped h1:3 (bad)", // a host:id, not a token
	} {
		if got := colorizeReportLine(theme, line); got != line {
			t.Errorf("colorizeReportLine(%q) = %q, want unchanged", line, got)
		}
	}
}

// TestColorizeTableStylesEveryColumn pins task m83's per-column styling and
// that padding stays outside the escapes, including a row with empty tags
// and descr and a host name with multi-byte runes (tabwriter aligns on
// runes, so byte offsets would mis-slice that row).
func TestColorizeTableStylesEveryColumn(t *testing.T) {
	theme := enabledTheme(t)
	rows := []worktime.Row{
		{Address: "host1:1", Entry: worktime.Entry{Host: "host1", ID: 1, Action: "add", Epoch: 1700000000, Value: 3600, Tags: []string{"work", "x"}, Descr: "two  spaces work:1.00h"}},
		{Address: "höst:22", Entry: worktime.Entry{Host: "höst", ID: 22, Action: "logout", Epoch: 1700000100}},
	}
	plain := worktime.FormatTable(rows)
	styled := colorizeTable(theme, plain)

	for _, want := range []string{
		"\x1b[36mhost1:1\x1b[0m  \x1b[35madd\x1b[0m     \x1b[34m",
		"\x1b[1m3600\x1b[0m   \x1b[33mwork,x\x1b[0m  two  spaces work:1.00h\n",
		"\x1b[36mhöst:22\x1b[0m  \x1b[35mlogout\x1b[0m  \x1b[34m",
		"\x1b[1m0\x1b[0m      ",
	} {
		if !strings.Contains(styled, want) {
			t.Errorf("styled table missing %q:\n%q", want, styled)
		}
	}
	if got := stripSGR.ReplaceAllString(styled, ""); got != plain {
		t.Errorf("colorized visible text differs from plain:\n got %q\nwant %q", got, plain)
	}
}

// TestColorizeTableMisalignedRowFallsBackToAddress is the negative case for
// column slicing: a descr containing a newline makes FormatTable emit a
// continuation line that is not a row at all. It must not be sliced at the
// header's offsets (which would style arbitrary descr text as an action or
// timestamp), and the visible text must still be untouched.
func TestColorizeTableMisalignedRowFallsBackToAddress(t *testing.T) {
	theme := enabledTheme(t)
	rows := []worktime.Row{
		{Address: "host1:1", Entry: worktime.Entry{Host: "host1", ID: 1, Action: "add", Epoch: 1700000000, Tags: []string{"work"}, Descr: "first\nsecond line of the description, long enough to reach every column"}},
	}
	plain := worktime.FormatTable(rows)
	styled := colorizeTable(theme, plain)

	lines := strings.Split(styled, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected a continuation line, got %q", styled)
	}
	if strings.ContainsRune(lines[2], '\x1b') {
		t.Errorf("continuation line was styled: %q", lines[2])
	}
	if got := stripSGR.ReplaceAllString(styled, ""); got != plain {
		t.Errorf("colorized visible text differs from plain:\n got %q\nwant %q", got, plain)
	}
}

// TestColorizeTableUnknownHeaderOnlyStylesAddresses guards the other
// fallback: without FormatTable's header there are no trustworthy offsets.
func TestColorizeTableUnknownHeaderOnlyStylesAddresses(t *testing.T) {
	theme := enabledTheme(t)
	table := "SOMETHING ELSE\nhost1:1  add  whatever\n"
	want := "\x1b[1mSOMETHING ELSE\x1b[0m\n\x1b[36mhost1:1\x1b[0m  add  whatever\n"
	if got := colorizeTable(theme, table); got != want {
		t.Errorf("colorizeTable:\n got %q\nwant %q", got, want)
	}
}

// TestLineStyleWriterStylesEachLine pins that a multi-line write is styled
// line by line (no escape sequence spans a newline) with the role picked per
// line, and that an unpicked line stays plain.
func TestLineStyleWriterStylesEachLine(t *testing.T) {
	var buf strings.Builder
	w := newLineStyleWriter(&buf, enabledTheme(t), func(line string) string {
		if strings.HasPrefix(line, "bad") {
			return ansi.RoleError
		}
		if strings.HasPrefix(line, "plain") {
			return ""
		}
		return ansi.RoleWarn
	})
	const text = "one\nbad two\nplain three\n"
	n, err := w.Write([]byte(text))
	if err != nil || n != len(text) {
		t.Fatalf("Write = (%d, %v), want (%d, nil)", n, err, len(text))
	}
	want := "\x1b[33mone\x1b[0m\n\x1b[31mbad two\x1b[0m\nplain three\n"
	if buf.String() != want {
		t.Errorf("styled lines:\n got %q\nwant %q", buf.String(), want)
	}
}

// TestLineStyleWriterDisabledReturnsUnderlyingWriter: with colors off the
// helper must not even wrap the writer, so plain output cannot change.
func TestLineStyleWriterDisabledReturnsUnderlyingWriter(t *testing.T) {
	var buf strings.Builder
	if w := newColorizeWriter(&buf, disabledTheme(t), ansi.RoleWarn); w != &buf {
		t.Errorf("disabled theme wrapped the writer: %T", w)
	}
}

func TestMigrateLineRole(t *testing.T) {
	cases := map[string]string{
		"migrated 2 host(s), 5 entries, 1 finding(s)": ansi.RoleSuccess,
		"  succeeded: a, b":                           ansi.RoleSuccess,
		"  failed: 1 host(s) -- retry with --force":   ansi.RoleError,
		"    - c: disk full":                          ansi.RoleError,
		"host a: entry 3 has unknown action":          ansi.RoleWarn,
	}
	for line, want := range cases {
		if got := migrateLineRole(line); got != want {
			t.Errorf("migrateLineRole(%q) = %q, want %q", line, got, want)
		}
	}
}

// TestFormatEntryStylesFieldsAndStripsToPlain checks --verbose entry
// rendering: each value carries its column's role, and the visible text is
// exactly what the disabled theme renders (descr quoting included).
func TestFormatEntryStylesFieldsAndStripsToPlain(t *testing.T) {
	entry := worktime.Entry{Host: "h1", ID: 7, Action: "add", Epoch: 1700000000, Value: 3600, Tags: []string{"work"}, Descr: `say "hi"`}
	plain := formatEntry(entry, disabledTheme(t))
	styled := formatEntry(entry, enabledTheme(t))

	if strings.ContainsRune(plain, '\x1b') {
		t.Fatalf("disabled theme emitted escapes: %q", plain)
	}
	if !strings.HasSuffix(plain, ` value=3600 tags=work descr="say \"hi\""`) {
		t.Errorf("plain rendering changed shape: %q", plain)
	}
	for _, want := range []string{
		"\x1b[36mh1:7\x1b[0m",
		"\x1b[2maction=\x1b[0m\x1b[35madd\x1b[0m",
		"\x1b[2mvalue=\x1b[0m\x1b[1m3600\x1b[0m",
		"\x1b[2mtags=\x1b[0m\x1b[33mwork\x1b[0m",
	} {
		if !strings.Contains(styled, want) {
			t.Errorf("styled entry missing %q:\n%q", want, styled)
		}
	}
	if got := stripSGR.ReplaceAllString(styled, ""); got != plain {
		t.Errorf("visible text differs:\n got %q\nwant %q", got, plain)
	}

	// No descr: no trailing descr="" in either rendering.
	entry.Descr = ""
	if got := formatEntry(entry, enabledTheme(t)); strings.Contains(got, "descr") {
		t.Errorf("empty descr still rendered: %q", got)
	}
}

// TestEveryCommandColoredOutputStripsToPlain is the end-to-end guard for
// task m83's invariant across the command surface: with colors forced each
// command's output contains escapes, and stripping them yields byte for
// byte what the same command prints with colors off. Only commands that do
// not change the store are compared, so both runs see identical data.
func TestEveryCommandColoredOutputStripsToPlain(t *testing.T) {
	store := newScratchStore(t)
	dbDir := t.TempDir()
	for _, args := range [][]string{
		{"add", "1h", "--at", "2024-01-02T09:00:00Z", "-d", "first"},
		{"add", "30m", "lunch", "--at", "2024-01-06T09:00:00Z"},
		{"start", "--at", "2024-01-08T09:00:00Z"},
	} {
		if out, err := runWork(t, store, args...); err != nil {
			t.Fatalf("seed %v: %v (%s)", args, err, out)
		}
	}
	host := currentHost(t)

	commands := [][]string{
		{"report"},
		{"report", "--verbose"},
		{"list"},
		{"search", "first"},
		{"status"},
		{"delete", "--dry-run", host + ":1"},
		{"export", "--db", dbDir},
	}
	for _, args := range commands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Setenv("CLICOLOR_FORCE", "")
			plain, err := runWork(t, store, args...)
			if err != nil {
				t.Fatalf("plain run: %v (%s)", err, plain)
			}
			if strings.ContainsRune(plain, '\x1b') {
				t.Fatalf("plain run emitted escapes: %q", plain)
			}
			t.Setenv("CLICOLOR_FORCE", "1")
			styled, err := runWork(t, store, args...)
			if err != nil {
				t.Fatalf("forced run: %v (%s)", err, styled)
			}
			if !strings.ContainsRune(styled, '\x1b') {
				t.Errorf("forced run has no escapes: %q", styled)
			}
			if got := stripSGR.ReplaceAllString(styled, ""); got != plain {
				t.Errorf("visible text differs:\n got %q\nwant %q", got, plain)
			}
			// The off switches must each win over CLICOLOR_FORCE.
			if out, _ := runWork(t, store, append(args, "--no-color")...); strings.ContainsRune(out, '\x1b') {
				t.Errorf("--no-color still emitted escapes: %q", out)
			}
			t.Setenv("NO_COLOR", "1")
			if out, _ := runWork(t, store, args...); strings.ContainsRune(out, '\x1b') {
				t.Errorf("NO_COLOR still emitted escapes: %q", out)
			}
		})
	}
}

// TestDeleteConfirmationPromptStyled covers the one interactive surface:
// the prompt is styled when forced, strips to the plain prompt, and a
// declined (empty) answer still deletes nothing.
func TestDeleteConfirmationPromptStyled(t *testing.T) {
	store := newScratchStore(t)
	for range 2 {
		if _, err := runWork(t, store, "add", "1h"); err != nil {
			t.Fatalf("seed add: %v", err)
		}
	}
	host := currentHost(t)
	args := []string{"delete", host + ":1", host + ":2"}

	plain, err := runWorkWithStdin(t, store, "n\n", args...)
	if err != nil {
		t.Fatalf("plain delete: %v", err)
	}
	t.Setenv("CLICOLOR_FORCE", "1")
	styled, err := runWorkWithStdin(t, store, "", args...)
	if err != nil {
		t.Fatalf("forced delete: %v", err)
	}
	if !strings.Contains(styled, "\x1b[33mdelete 2 entries\x1b[0m (\x1b[36m"+host+":1\x1b[0m, ") {
		t.Errorf("prompt not styled: %q", styled)
	}
	if got := stripSGR.ReplaceAllString(styled, ""); got != plain {
		t.Errorf("visible prompt differs:\n got %q\nwant %q", got, plain)
	}
	if got := len(readEntries(t, store, host)); got != 2 {
		t.Errorf("declined delete left %d entries, want 2", got)
	}
}

// TestMigrateReportStyledWhenForced: migrate resolves its own theme (it
// bypasses newRuntime), so it gets its own forced/plain/--no-color check.
func TestMigrateReportStyledWhenForced(t *testing.T) {
	run := func(t *testing.T, extra ...string) string {
		t.Helper()
		dbDir := t.TempDir()
		storeDir := t.TempDir()
		writeLegacyFixture(t, dbDir, "mhost", `{"action":"add","what":"work","epoch":100,"source":"mhost","human":"h","value":3600}`)
		out, err := runWork(t, storeDir, append([]string{"migrate", "--db", dbDir}, extra...)...)
		if err != nil {
			t.Fatalf("migrate: %v (%s)", err, out)
		}
		return out
	}
	newScratchStore(t) // hermetic color/config environment
	plain := run(t)
	if strings.ContainsRune(plain, '\x1b') {
		t.Fatalf("plain migrate emitted escapes: %q", plain)
	}
	t.Setenv("CLICOLOR_FORCE", "1")
	styled := run(t)
	if !strings.HasPrefix(styled, "\x1b[32mmigrated 1 host(s)") {
		t.Errorf("migrate summary not green: %q", styled)
	}
	if got := stripSGR.ReplaceAllString(styled, ""); got != plain {
		t.Errorf("visible text differs:\n got %q\nwant %q", got, plain)
	}
	if out := run(t, "--no-color"); strings.ContainsRune(out, '\x1b') {
		t.Errorf("--no-color still emitted escapes: %q", out)
	}
}
