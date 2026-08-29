package worktime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"
)

// Filter selects a subset of an in-memory entry set — a single host's
// (Store.Entries) or every host's (CollectEntries) — by host, tag, action,
// description substring, time range, and value range.
//
// Every field is optional and defaults to "no constraint": the zero Filter
// matches every entry. String fields empty, time fields zero, and pointer
// fields nil all mean "don't filter on this". Min/Max are pointers (rather
// than plain int64) so a bound of exactly 0 is distinguishable from "not
// set" — a duration filter of "value >= 0" is a real, useful query (e.g.
// excluding Sub withdrawals).
type Filter struct {
	// Host matches Entry.Host exactly. Hosts are directory-derived names
	// (see normalizeHost), so comparison is case-sensitive like the rest
	// of the store.
	Host string
	// Tag matches when it appears anywhere in Entry.Tags. Comparison is
	// case-sensitive, matching how tags are compared everywhere else in
	// this package (ClassifyTag, AccountingTag, normalizeTags — none of
	// them fold case).
	Tag string
	// Action matches Entry.Action case-insensitively: ValidateEntry itself
	// lowercases before comparing against the fixed login/logout/add set,
	// so the domain already treats action as case-insensitive.
	Action string
	// Descr matches when it appears as a substring of Entry.Descr,
	// case-insensitively. Unlike Host/Tag, Descr is free-text a human
	// typed, so folding case is what makes "search" behave like search
	// rather than grep.
	Descr string
	// Since is an inclusive lower bound on the entry's time (derived from
	// Entry.Epoch). Zero means no lower bound.
	Since time.Time
	// Until is an inclusive upper bound on the entry's time. Zero means
	// no upper bound.
	Until time.Time
	// Min is an inclusive lower bound on Entry.Value. Nil means no bound.
	Min *int64
	// Max is an inclusive upper bound on Entry.Value. Nil means no bound.
	Max *int64
	// Limit caps the number of returned rows. Zero or negative means
	// unlimited: a caller building a Filter from CLI flags with an unset
	// --limit ends up with the zero value, which should mean "no cap",
	// not "return nothing".
	Limit int
}

// Row pairs an Entry with the "<host>:<id>" address ParseAddress accepts,
// so query output can be pasted straight into Modify/Delete without the
// caller re-deriving it.
type Row struct {
	Address string
	Entry   Entry
}

// jsonRow is FormatJSON's wire shape: Entry's own field order (see
// model.go) with "address" prepended, since that is the one derived field
// query output adds on top of a stored entry.
type jsonRow struct {
	Address string   `json:"address"`
	ID      int64    `json:"id"`
	Action  string   `json:"action"`
	Epoch   int64    `json:"epoch"`
	Host    string   `json:"host"`
	Value   int64    `json:"value,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Descr   string   `json:"descr,omitempty"`
}

// Query filters entries against f and returns matching rows in the same
// relative order as entries, capped at f.Limit. Callers typically pass
// CollectEntries(store) for a cross-host search or store.Entries(host) to
// scope a search to one host; Query itself does no sorting, so the order
// of its output follows the order of its input.
func Query(entries []Entry, f Filter) ([]Row, error) {
	if err := f.Validate(); err != nil {
		return nil, fmt.Errorf("invalid filter: %w", err)
	}

	rows := make([]Row, 0, len(entries))
	for _, entry := range entries {
		if !f.Match(entry) {
			continue
		}
		rows = append(rows, Row{Address: address(entry), Entry: entry})
		if f.Limit > 0 && len(rows) >= f.Limit {
			break
		}
	}
	return rows, nil
}

// Validate reports whether f's bounds are internally consistent. An
// inverted range (Since after Until, or Min above Max) can only ever match
// nothing, which is far more likely a caller mistake (e.g. swapped flags)
// than an intentional empty query, so Query rejects it up front rather than
// silently returning zero rows.
func (f Filter) Validate() error {
	if !f.Since.IsZero() && !f.Until.IsZero() && f.Since.After(f.Until) {
		return fmt.Errorf("since %s is after until %s", f.Since, f.Until)
	}
	if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
		return fmt.Errorf("min %d is greater than max %d", *f.Min, *f.Max)
	}
	return nil
}

// Match reports whether entry satisfies every constraint f sets. Each
// constraint is delegated to its own predicate so Match itself stays a
// flat readable list rather than one long condition.
func (f Filter) Match(entry Entry) bool {
	return matchesHost(entry, f.Host) &&
		matchesTag(entry, f.Tag) &&
		matchesAction(entry, f.Action) &&
		matchesDescr(entry, f.Descr) &&
		matchesTimeRange(entry, f.Since, f.Until) &&
		matchesValueRange(entry, f.Min, f.Max)
}

// CollectEntries gathers every entry across every host in store, sorted by
// epoch (ties broken by id). This is entries.go's allEntriesSorted helper
// under a query-facing name: that function already does exactly this merge
// for login/logout replay, and a second copy of the same sort here would
// only be able to drift from it.
func CollectEntries(store *Store) []Entry {
	return allEntriesSorted(store)
}

// FormatTable renders rows as a whitespace-aligned table (address, action,
// time, value, tags, descr) with a header line. An empty rows slice still
// renders the header, so a caller piping output somewhere always sees a
// consistent shape rather than having to special-case "no results".
//
// tabwriter.Writer.Write/Flush return an error in their signature, but the
// underlying sink here is an in-memory bytes.Buffer, whose Write never
// fails; the errors are discarded explicitly (rather than left unchecked)
// so errcheck can confirm that, not just assume it.
func FormatTable(rows []Row) string {
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ADDRESS\tACTION\tWHEN\tVALUE\tTAGS\tDESCR")
	for _, row := range rows {
		writeTableRow(tw, row)
	}
	_ = tw.Flush()
	return buf.String()
}

// FormatJSON renders rows as an indented JSON array. Each element carries
// the same fields as Entry's on-disk JSON shape plus a leading "address",
// so output can be piped into another tool without it having to re-derive
// "<host>:<id>" itself. An empty rows slice renders "[]", not "null".
func FormatJSON(rows []Row) (string, error) {
	out := make([]jsonRow, len(rows))
	for i, row := range rows {
		out[i] = jsonRow{
			Address: row.Address,
			ID:      row.Entry.ID,
			Action:  row.Entry.Action,
			Epoch:   row.Entry.Epoch,
			Host:    row.Entry.Host,
			Value:   row.Entry.Value,
			Tags:    row.Entry.Tags,
			Descr:   row.Entry.Descr,
		}
	}

	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal query rows: %w", err)
	}
	return string(payload), nil
}

func matchesHost(entry Entry, host string) bool {
	return host == "" || entry.Host == host
}

func matchesTag(entry Entry, tag string) bool {
	if tag == "" {
		return true
	}
	for _, t := range entry.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

func matchesAction(entry Entry, action string) bool {
	return action == "" || strings.EqualFold(entry.Action, action)
}

func matchesDescr(entry Entry, substr string) bool {
	return substr == "" || strings.Contains(strings.ToLower(entry.Descr), strings.ToLower(substr))
}

func matchesTimeRange(entry Entry, since, until time.Time) bool {
	at := time.Unix(entry.Epoch, 0)
	if !since.IsZero() && at.Before(since) {
		return false
	}
	if !until.IsZero() && at.After(until) {
		return false
	}
	return true
}

func matchesValueRange(entry Entry, min, max *int64) bool {
	if min != nil && entry.Value < *min {
		return false
	}
	if max != nil && entry.Value > *max {
		return false
	}
	return true
}

// address renders entry's "<host>:<id>" address, the same shape
// ParseAddress parses back.
func address(entry Entry) string {
	return fmt.Sprintf("%s:%d", entry.Host, entry.ID)
}

// writeTableRow renders one row's tab-separated columns; kept separate from
// FormatTable so the header/loop/flush shape there stays readable.
func writeTableRow(tw *tabwriter.Writer, row Row) {
	when := time.Unix(row.Entry.Epoch, 0).Format("2006-01-02 15:04:05")
	tags := strings.Join(row.Entry.Tags, ",")
	_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n",
		row.Address, row.Entry.Action, when, row.Entry.Value, tags, row.Entry.Descr)
}
