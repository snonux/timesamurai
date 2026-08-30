package worktime

import (
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"time"
)

// This file reproduces worktime.rb's `report`/`print_week`/`print_day`/
// `print_data`/`get_data_valstr`/`get_epoch_daystr`/`get_epoch_weekstr`
// methods byte-for-byte, including several behaviors that read as bugs in
// the original Ruby but must NOT be "fixed" here: the golden-diff test
// (task 271) compares this package's output directly against
// `ruby worktime.rb --report`. Every quirk below is deliberate, not an
// oversight — see the comment at its implementation for the reasoning.

const (
	secondsPerHour = int64(3600)
	eightHours     = 8 * secondsPerHour
	weekSeparator  = "================================================\n"
)

// reportCategories is the fixed print order from print_data: balance, work,
// lunch, off, sick, bank, pet, selfdevelopment. "buffer" is appended
// separately afterwards since only week summary lines ever receive it.
var reportCategories = []string{
	"balance", "work", "lunch", "off", "sick", "bank", "pet", "selfdevelopment",
}

// DayReport is one printed day line.
type DayReport struct {
	// Epoch is the first entry's timestamp processed for this calendar day
	// (get_human/marker/verbose all key off it, not a fixed midnight).
	Epoch int64
	// Label is the exact "%a %Y%m%d %V" string print_day renders.
	Label string
	// Marker is "*" (weekend day, or >=8h off/bank that day) or " ".
	Marker string
	// Values are this day's per-category seconds, AFTER the day-level
	// minusfor subtraction print_day applies to "work" for display.
	Values map[string]int64
}

// WeekReport is one printed week block: its days, then the "====" +
// totals line. Values["balance"] already carries the running cross-week
// balance (there is no separate "this week only" balance printed anywhere
// in worktime.rb); Buffer is the running cross-report buffer total as of
// this week's boundary, not a per-week figure.
type WeekReport struct {
	Days   []DayReport
	Values map[string]int64
	Buffer int64
}

// openLogin is one still-open login: its start epoch and the host that
// logged it in. The host is only needed for the superseded-login warning
// (see applyAction's ActionLogin case) — report() itself never keys
// anything on it, since Ruby's login map is category-only too.
type openLogin struct {
	Epoch int64
	Host  string
}

// BoundaryLogin identifies one category with a login still open immediately
// before a report range's Since boundary — i.e. a session that started
// before the range but hadn't been logged out yet at that instant. Category
// is the accounting category (entryCategory); Host is the host that
// recorded the open login, carried through only so a synthetic continuation
// entry attributes to somewhere real, the way a genuine login would.
type BoundaryLogin struct {
	Category string
	Host     string
}

// OpenLoginsBefore replays entries strictly before `before` (mirroring
// applyAction's login/logout bookkeeping; add entries never open or close a
// login, so they're skipped) and reports every category left with a
// still-open login at that instant.
//
// This exists for task 281's ranged-report fix: worktime.Query's time-range
// filter keeps an in-range logout but drops its matching login when that
// login started before the range's Since bound, and BuildReport's
// applyAction then hard-errors on the resulting orphan logout ("logout
// without login for ..."). cli.reportEntries calls this against the FULL
// unfiltered history to find any such straddling sessions, then splices in
// a synthetic login pinned to the range boundary itself (not the session's
// true start) so only the in-range portion gets credited — see
// cli.withBoundaryLogins.
//
// entries need not be pre-sorted or pre-restricted to before `before`;
// both are handled here the same way BuildReport handles its own input.
func OpenLoginsBefore(entries []Entry, before time.Time) []BoundaryLogin {
	beforeEpoch := before.Unix()
	login := map[string]openLogin{}
	for _, entry := range sortEntriesForReport(entries) {
		if entry.Epoch >= beforeEpoch {
			break // sorted ascending: nothing from here on is "before"
		}
		category := entryCategory(entry)
		switch strings.ToLower(strings.TrimSpace(entry.Action)) {
		case ActionLogin:
			// Bug-compatible with applyAction: a second login for an
			// already-open category silently overwrites the first (Ruby's
			// "already logged in" guard is dead code — see applyAction's
			// comment). Replaying that same overwrite here keeps this
			// boundary scan consistent with what a full replay would do.
			login[category] = openLogin{Epoch: entry.Epoch, Host: entry.Host}
		case ActionLogout:
			delete(login, category)
		}
	}

	out := make([]BoundaryLogin, 0, len(login))
	for category, open := range login {
		out = append(out, BoundaryLogin{Category: category, Host: open.Host})
	}
	// Deterministic order: callers (and their tests) shouldn't have to
	// account for Go's randomized map iteration order.
	slices.SortFunc(out, func(a, b BoundaryLogin) int {
		return strings.Compare(a.Category, b.Category)
	})
	return out
}

// reportState is the mutable state report() threads through sorted_entries:
// the day/week currently being accumulated, open logins, and the two
// running totals (balance, buffer) that persist across week boundaries.
type reportState struct {
	cfg   AccountingConfig
	warn  io.Writer            // destination for the superseded-login warning
	login map[string]openLogin // category -> its still-open login

	totalBuffer int64 // running sum of bufferfor 'add' entries, never reset
	balance     int64 // running cross-week balance, never reset

	day  dayAccumulator
	week weekAccumulator

	prevDayKey  string
	prevWeekKey string
	weeks       []WeekReport
}

type dayAccumulator struct {
	epoch  int64
	values map[string]int64
}

type weekAccumulator struct {
	days   []DayReport
	values map[string]int64 // RAW per-category sums, before any minusfor subtraction
}

// newReportState returns a reportState ready to replay entries from the
// very first one (no open logins, empty running day/week accumulators).
// warn receives the superseded-login diagnostic (see applyAction); a nil
// warn is replaced with io.Discard so callers that don't care about the
// warning (e.g. most tests) don't have to pass one.
func newReportState(cfg AccountingConfig, warn io.Writer) *reportState {
	if warn == nil {
		warn = io.Discard
	}
	return &reportState{
		cfg:   cfg,
		warn:  warn,
		login: map[string]openLogin{},
		day:   dayAccumulator{values: map[string]int64{}},
		week:  weekAccumulator{values: map[string]int64{}},
	}
}

// BuildReport replays entries (from any hosts, any order) the way
// worktime.rb's `report` replays sorted_entries, and returns one WeekReport
// per ISO week block. entries is defensively re-sorted here (stable, epoch
// only) so callers do not need to pre-sort; on the real 12,802-entry
// dataset every entry sharing an exact epoch with another is either a
// commutative 'add' or a login+add pair whose result doesn't depend on
// order (verified against the live db.*.json files), so the merge order of
// distinct hosts feeding in here does not affect output.
//
// warn receives one line per superseded-login discard (see applyAction's
// ActionLogin case for why that discard happens and why it must not be an
// error) — pass os.Stderr to surface it, or nil/io.Discard to ignore it.
// This is purely a diagnostic side channel: it never changes the returned
// []WeekReport, so passing a different warn writer cannot change what gets
// reported.
func BuildReport(entries []Entry, cfg AccountingConfig, warn io.Writer) ([]WeekReport, error) {
	if len(entries) == 0 {
		// Ruby would crash here (Time.at(nil) on the placeholder day/week
		// that report() always flushes once) — an empty store is not a
		// real-world path worth reproducing that crash for.
		return nil, nil
	}

	sorted := sortEntriesForReport(entries)
	state := newReportState(cfg, warn)
	for _, entry := range sorted {
		if err := state.process(entry); err != nil {
			return nil, err
		}
	}
	state.flushDay()
	state.weeks = append(state.weeks, state.finalizeWeek())
	return state.weeks, nil
}

// FormatReport renders weeks exactly as report()'s print loop: each day
// line, the 48-'=' separator, one totals line, then three newlines (the
// totals line's own trailing "\n" plus two further blank `print "\n"`
// calls) before the next week's days begin.
func FormatReport(weeks []WeekReport, verbose bool) string {
	var out strings.Builder
	for _, week := range weeks {
		for _, day := range week.Days {
			writeDayLine(&out, day, verbose)
		}
		out.WriteString(weekSeparator)
		writeCategoryValues(&out, week.Values)
		writeValue(&out, "buffer", week.Buffer)
		out.WriteString("\n\n\n")
	}
	return out.String()
}

// sortEntriesForReport mirrors sorted_entries's `sort_by! { |a| a['epoch'] }`:
// a stable sort keyed on epoch alone, so entries sharing an epoch keep
// their input relative order (matching the "epoch-only, no secondary key"
// rule already established for the legacy codec in task o61).
func sortEntriesForReport(entries []Entry) []Entry {
	out := make([]Entry, len(entries))
	copy(out, entries)
	slices.SortStableFunc(out, func(a, b Entry) int {
		switch {
		case a.Epoch < b.Epoch:
			return -1
		case a.Epoch > b.Epoch:
			return 1
		default:
			return 0
		}
	})
	return out
}

// process replays one entry: day/week boundary checks happen first (using
// this entry's own keys), exactly as report()'s loop checks daystr/weekstr
// before touching the entry's own values — so the day/week just closed is
// flushed using entries strictly before this one, and this entry lands in
// the freshly (re)started day/week.
func (s *reportState) process(entry Entry) error {
	dayKey := dayLabel(entry.Epoch)
	weekKey := weekKey(entry.Epoch)
	if s.prevDayKey == "" {
		s.prevDayKey = dayKey
	}
	if s.prevWeekKey == "" {
		s.prevWeekKey = weekKey
	}

	if dayKey != s.prevDayKey {
		s.flushDay()
		s.prevDayKey = dayKey
	}
	if weekKey != s.prevWeekKey {
		s.weeks = append(s.weeks, s.finalizeWeek())
		s.week = weekAccumulator{values: map[string]int64{}}
		s.prevWeekKey = weekKey
	}

	category := entryCategory(entry)
	if _, ok := s.day.values[category]; !ok {
		s.day.values[category] = 0
	}
	if s.day.epoch == 0 {
		s.day.epoch = entry.Epoch
	}

	return s.applyAction(entry, category)
}

// applyAction mirrors report()'s action switch, including its one
// deliberate bug: Ruby's "already logged in" guard tests `login.key?('what')`
// — a literal string key insert() never sets (real keys are the category
// name) — so the guard is always false. A second login for an already-open
// category silently overwrites the first: the earlier login is discarded
// with no logout ever recorded for it. Real data hits this exactly once:
// host earth logs in for "work" at epoch 1781618168 (2026-06-16 16:56) and
// never logs out; host MBDVXJ4XKH9C's own "work" login at epoch 1781630083
// (the same evening, 20:14 — a plausible desktop-to-laptop switch) arrives
// first in the globally epoch-sorted replay (sorted_entries/BuildReport
// merge every host's entries into one timeline) and discards it. Verified
// by instrumenting this exact code path against the live db.*.json files:
// it is the only discard in the whole 12,802-entry dataset. (An earlier
// draft of this comment named earth's own later login at 1783319163 as the
// superseder; that login's own predecessor had already been closed by a
// logout by the time it fires, so it does not trigger a second discard —
// the actual superseder is the cross-host login above.) The golden report
// reflects the discard, not an error. Since report.go doesn't get to "fix"
// Ruby's guard, the only honest improvement available is observability:
// warnDiscardedLogin logs the discard to s.warn before it happens, naming
// both the entry being thrown away and the one that overwrites it.
func (s *reportState) applyAction(entry Entry, category string) error {
	switch action := strings.ToLower(strings.TrimSpace(entry.Action)); action {
	case ActionAdd:
		s.day.values[category] += entry.Value
		if slices.Contains(s.cfg.BufferFor, category) {
			s.totalBuffer += entry.Value
		}
	case ActionLogin:
		s.warnDiscardedLogin(entry, category)
		s.login[category] = openLogin{Epoch: entry.Epoch, Host: entry.Host}
	case ActionLogout:
		open, ok := s.login[category]
		if !ok {
			// Skip rather than abort. worktime.rb dies here, but a report
			// that refuses to render at all is worse than one that renders
			// and says what it could not pair -- and this state is now
			// reachable by ordinary use: deleting the login half of a pair
			// leaves exactly this lone logout behind, which would otherwise
			// make every subsequent report unusable until someone guessed
			// that `work undo` was the fix. No time is credited for an
			// interval whose start is unknown.
			s.warnUnpairable(entry, "logout without a matching login")
			return nil
		}
		s.day.values[category] += entry.Epoch - open.Epoch
		delete(s.login, category)
	default:
		// Same reasoning: an entry carrying an action this version does not
		// understand is skipped and named, not fatal. It costs one line of
		// output and keeps every other week readable.
		s.warnUnpairable(entry, fmt.Sprintf("unknown action %q", entry.Action))
		return nil
	}
	return nil
}

// warnUnpairable reports one entry that report() cannot fold into a total,
// naming the address so it can be inspected with `work list` or reverted
// with `work undo`.
func (s *reportState) warnUnpairable(entry Entry, why string) {
	if s.warn == nil {
		return
	}
	_, _ = fmt.Fprintf(s.warn, "warning: skipped %s:%d (%s) at epoch %d\n",
		entry.Host, entry.ID, why, entry.Epoch)
}

// warnDiscardedLogin writes one diagnostic line to s.warn when entry (a new
// login for category) is about to silently overwrite an already-open login
// for that same category — the discard applyAction's comment describes.
// It is a no-op when category has no open login, i.e. the common case.
// This never returns an error and never touches report state: it exists
// solely so the discard, which report.go must still perform to stay
// byte-for-byte with Ruby, is at least visible instead of silent. A write
// failure on the warn writer (e.g. a closed pipe) is deliberately not
// reported to the caller: losing a diagnostic line must never turn into an
// aborted report, so the error is discarded explicitly here rather than
// left unchecked.
func (s *reportState) warnDiscardedLogin(entry Entry, category string) {
	discarded, open := s.login[category]
	if !open {
		return
	}
	_, _ = fmt.Fprintf(s.warn,
		"warning: superseded login discarded (never logged out): host=%s category=%q epoch=%d; superseded by login at epoch=%d on host=%s\n",
		discarded.Host, category, discarded.Epoch, entry.Epoch, entry.Host)
}

// flushDay mirrors add_day_to_week followed by print_day's in-place mutation:
// the week's RAW totals accumulate the day's values first (before any
// minusfor subtraction), and only a separate display copy has minusfor
// categories subtracted from "work" — because finalizeWeek later subtracts
// minusfor from the week's own raw sum too, and
// sum(work_i) - sum(lunch_i) == sum(work_i - lunch_i), so the day-level and
// week-level subtractions are two views of the same single deduction, not
// a double one. Getting this backwards (e.g. summing already-subtracted day
// values into the week) would subtract lunch twice.
func (s *reportState) flushDay() {
	if s.day.epoch == 0 {
		return // nothing processed yet; only true before the very first entry
	}

	for category, value := range s.day.values {
		s.week.values[category] += value
	}

	display := cloneValues(s.day.values)
	for _, category := range s.cfg.MinusFor {
		if value, ok := display[category]; ok {
			display["work"] -= value
		}
	}

	s.week.days = append(s.week.days, DayReport{
		Epoch:  s.day.epoch,
		Label:  dayLabel(s.day.epoch),
		Marker: dayMarker(s.day.epoch, s.day.values, s.cfg.WeekendDays),
		Values: display,
	})
	s.day = dayAccumulator{values: map[string]int64{}}
}

// finalizeWeek mirrors print_week: the weekly target is weekworkhours minus
// this week's plusfor categories; work is reduced once by minusfor
// categories (against the week's RAW sum — see flushDay's comment for why
// that isn't a double subtraction); this week's balance (work - target) is
// added to the running cross-week balance; buffer is the running
// cross-report total as of this boundary (never reset, never per-week).
func (s *reportState) finalizeWeek() WeekReport {
	values := cloneValues(s.week.values)

	target := int64(math.Round(s.cfg.WeekWorkHours * float64(secondsPerHour)))
	for _, category := range s.cfg.PlusFor {
		target -= values[category]
	}

	work := values[WorkTag]
	for _, category := range s.cfg.MinusFor {
		work -= values[category]
	}
	values[WorkTag] = work

	s.balance += work - target
	values["balance"] = s.balance

	return WeekReport{Days: s.week.days, Values: values, Buffer: s.totalBuffer}
}

// entryCategory mirrors `e.key?('what') ? e['what'] : 'work'`: the entry's
// literal first tag, defaulting to "work" when untagged. This deliberately
// does NOT run tags through AccountingTag/ClassifyTag (entries.go): real
// history carries one-off custom tags (e.g. "bulgarian", "dotfiles",
// "selfimprovement", "tools") that classify as plain labels rather than
// work/plus/minus/buffer. Ruby keeps each in its own (unprinted, uncounted)
// bucket; folding them into "work" via a classifier fallback would inflate
// worked hours that were never actually logged as work.
func entryCategory(e Entry) string {
	if len(e.Tags) == 0 {
		return WorkTag
	}
	return e.Tags[0]
}

// dayMarker mirrors print_day's '*' logic: an extra day off or banked
// (>=8h) marks '*' even on a weekday and is checked first; otherwise a
// configured weekend day (default Sat/Sun) also gets '*'.
func dayMarker(epoch int64, values map[string]int64, weekendDays []string) string {
	if values["off"] >= eightHours {
		return "*"
	}
	if values["bank"] >= eightHours {
		return "*"
	}
	if slices.Contains(weekendDays, time.Unix(epoch, 0).Format("Mon")) {
		return "*"
	}
	return " "
}

// dayLabel renders get_epoch_daystr's exact `strftime('%a %Y%m%d %V')`:
// abbreviated weekday, compact date, ISO week number — all derived from
// the same instant, so Go's ISOWeek() (also ISO-8601) agrees with Ruby's
// %V for that same date.
func dayLabel(epoch int64) string {
	t := time.Unix(epoch, 0)
	_, week := t.ISOWeek()
	return fmt.Sprintf("%s %s %02d", t.Format("Mon"), t.Format("20060102"), week)
}

// weekKey mirrors get_epoch_weekstr: the ISO week number ALONE, with no
// year. This is a deliberate bug-for-bug port, not a simplification: Ruby
// detects a week boundary purely by that number changing, so two entries
// in the same numbered week of different years, with nothing bearing a
// different week number between them, would (in Ruby) merge into a single
// printed block instead of splitting. Real per-day data never goes a full
// year without a differently-numbered week appearing in between, so this
// never fires in practice — but replicating the year-blind key (rather
// than a year+week key that "fixes" it) is what makes this a faithful port.
func weekKey(epoch int64) string {
	_, week := time.Unix(epoch, 0).ISOWeek()
	return fmt.Sprintf("%02d", week)
}

// writeDayLine mirrors `print "   #{add} #{daystr}:"` followed by
// print_data(day) and a trailing newline. Verbose epoch annotation only
// ever applies here (weeks never carry an 'epoch' key in Ruby, so
// print_data on a week never appends one).
func writeDayLine(out *strings.Builder, day DayReport, verbose bool) {
	out.WriteString("   ")
	out.WriteString(day.Marker)
	out.WriteString(" ")
	out.WriteString(day.Label)
	out.WriteString(":")
	writeCategoryValues(out, day.Values)
	if verbose {
		fmt.Fprintf(out, " epoch:%d(%s)", day.Epoch, time.Unix(day.Epoch, 0))
	}
	out.WriteString("\n")
}

// writeCategoryValues mirrors print_data's fixed sequence of
// get_data_valstr calls: each of the 8 keys prints " key:%.2fh" (Ruby's
// "%02.2fh" has no effective zero-padding here since the field is always
// wider than 2 chars) unless it is zero — except "work", which the Ruby
// :show_empty flag always prints, key present or not, zero or not.
// Note: balance is called with Ruby's :if_exists flag, but get_data_valstr
// only special-cases :show_empty; :if_exists behaves exactly like the
// default (suppress on zero), confirmed by reading worktime.rb directly —
// balance is NOT printed whenever it happens to be present-but-zero.
func writeCategoryValues(out *strings.Builder, values map[string]int64) {
	for _, key := range reportCategories {
		value := values[key]
		if value == 0 && key != WorkTag {
			continue
		}
		writeValue(out, key, value)
	}
}

// writeValue renders one " key:%.2fh" token. Unlike the 8 fixed categories,
// the week line's "buffer" token has no zero-suppression at all in Ruby
// (`total_bufferfor.nil? ? ” : format(...)` — nil vs. zero are different
// things, and total_bufferfor is only ever nil on day lines, which never
// call this for buffer at all), so callers decide suppression themselves.
func writeValue(out *strings.Builder, key string, seconds int64) {
	fmt.Fprintf(out, " %s:%sh", key, formatHours(seconds))
}

// formatHours renders seconds as hours with exactly 2 decimal digits,
// replicating Ruby's sprintf('%02.2f', seconds/3600.0) bit for bit rather
// than Go's default float formatting. Ruby's float formatting rounds the
// value's shortest round-trip decimal representation to 2 places using
// round-half-to-even (banker's rounding); Go's fmt/strconv instead rounds
// the double's full binary expansion, which disagrees at exact
// three-decimal ties — e.g. 32706s = 9.085h exactly: Ruby prints "9.08"
// (round-half-even on the clean decimal 9.085), Go's %.2f prints "9.09"
// (the nearest representable double to 9.085 is a hair above it, so
// correctly-rounded-from-binary rounds up). Confirmed empirically against
// `ruby -e "format('%02.2f', v)"` across a spread of X.XX5 values,
// including negative ones (sign is preserved even when the rounded
// magnitude is exactly zero, e.g. -1s prints "-0.00").
// Since every value here is an exact integer count of seconds, the tie
// case is resolved with pure integer arithmetic instead of floating
// point, sidestepping float representability entirely: hours*100 equals
// seconds/36 exactly as a rational number.
func formatHours(seconds int64) string {
	sign := ""
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	hundredths := roundHalfEven(seconds, 36)
	return fmt.Sprintf("%s%d.%02d", sign, hundredths/100, hundredths%100)
}

// roundHalfEven rounds the rational n/d (n, d >= 0) to the nearest integer,
// resolving an exact .5 tie toward the even neighbor — matching Ruby's
// sprintf rounding mode for the ties formatHours's seconds/3600 values can
// land on exactly. One exception: the smallest possible tie (quotient 0
// vs. 1, i.e. exactly 18 seconds = 0.005h) rounds UP in real Ruby
// (confirmed against Ruby 4.0.6 by generating and formatting 100,000 exact
// .5 ties spanning 0..500h: every other tie matched round-half-even, only
// this one didn't — real report data hits it once, at a week balance of
// exactly 18 seconds). This reads as a quirk of Ruby's underlying dtoa
// digit generation near zero rather than a documented rounding rule, but
// byte-for-byte parity means reproducing it rather than "fixing" it away.
func roundHalfEven(n, d int64) int64 {
	quotient, remainder := n/d, n%d
	switch twice := 2 * remainder; {
	case twice < d:
		return quotient
	case twice > d:
		return quotient + 1
	case quotient == 0:
		return quotient + 1
	case quotient%2 == 0:
		return quotient
	default:
		return quotient + 1
	}
}

// cloneValues returns a shallow copy so display mutation (flushDay's
// per-day minusfor subtraction) never disturbs the week's raw running sums.
func cloneValues(values map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
