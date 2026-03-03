package worktime

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	actionLogin  = "login"
	actionLogout = "logout"
	actionAdd    = "add"
)

// Login creates a login entry after validating the category is not already logged in.
func Login(dbDir, hostname, category string, at time.Time, descr string) (Entry, error) {
	host, err := normalizeHostname(hostname)
	if err != nil {
		return Entry{}, err
	}

	cat := normalizeCategory(category)
	loggedIn, err := isLoggedIn(dbDir, cat)
	if err != nil {
		return Entry{}, err
	}
	if loggedIn {
		return Entry{}, fmt.Errorf("already logged in for %q", cat)
	}

	entry := newEntry(actionLogin, host, cat, at, 0, descr)
	return appendHostEntry(dbDir, host, entry)
}

// Logout creates a logout entry after validating the category is currently logged in.
func Logout(dbDir, hostname, category string, at time.Time, descr string) (Entry, error) {
	host, err := normalizeHostname(hostname)
	if err != nil {
		return Entry{}, err
	}

	cat := normalizeCategory(category)
	loggedIn, err := isLoggedIn(dbDir, cat)
	if err != nil {
		return Entry{}, err
	}
	if !loggedIn {
		return Entry{}, fmt.Errorf("not logged in for %q", cat)
	}

	entry := newEntry(actionLogout, host, cat, at, 0, descr)
	return appendHostEntry(dbDir, host, entry)
}

// Add creates an add entry with a positive duration.
func Add(dbDir, hostname, category string, duration time.Duration, at time.Time, descr string) (Entry, error) {
	host, err := normalizeHostname(hostname)
	if err != nil {
		return Entry{}, err
	}
	if duration <= 0 {
		return Entry{}, errors.New("duration must be positive")
	}

	cat := normalizeCategory(category)
	entry := newEntry(actionAdd, host, cat, at, durationToSeconds(duration), descr)
	return appendHostEntry(dbDir, host, entry)
}

// Sub creates an add entry with a negative duration value.
func Sub(dbDir, hostname, category string, duration time.Duration, at time.Time, descr string) (Entry, error) {
	host, err := normalizeHostname(hostname)
	if err != nil {
		return Entry{}, err
	}
	if duration <= 0 {
		return Entry{}, errors.New("duration must be positive")
	}

	cat := normalizeCategory(category)
	entry := newEntry(actionAdd, host, cat, at, -durationToSeconds(duration), descr)
	return appendHostEntry(dbDir, host, entry)
}

// UseBuffer transfers duration from selfdevelopment to work.
func UseBuffer(dbDir, hostname string, duration time.Duration, at time.Time, descr string) ([]Entry, error) {
	if duration <= 0 {
		return nil, errors.New("duration must be positive")
	}

	removed, err := Sub(dbDir, hostname, "selfdevelopment", duration, at, descr)
	if err != nil {
		return nil, err
	}

	added, err := Add(dbDir, hostname, "work", duration, at, descr)
	if err != nil {
		return nil, err
	}

	return []Entry{removed, added}, nil
}

// EditEntry replaces an entry by index in the host database after validation.
func EditEntry(dbDir, hostname string, index int, replacement Entry) (Entry, error) {
	host, err := normalizeHostname(hostname)
	if err != nil {
		return Entry{}, err
	}

	db, err := LoadHost(dbDir, host)
	if err != nil {
		return Entry{}, err
	}

	entries := db.Entries[host]
	if index < 0 || index >= len(entries) {
		return Entry{}, fmt.Errorf("entry index %d out of range", index)
	}

	normalized, err := normalizeEditedEntry(replacement, host)
	if err != nil {
		return Entry{}, err
	}

	entries[index] = normalized
	db.Entries[host] = entries
	if err := SaveHost(dbDir, host, db); err != nil {
		return Entry{}, err
	}

	return normalized, nil
}

// DeleteEntry removes an entry by index from the host database.
func DeleteEntry(dbDir, hostname string, index int) (Entry, error) {
	host, err := normalizeHostname(hostname)
	if err != nil {
		return Entry{}, err
	}

	db, err := LoadHost(dbDir, host)
	if err != nil {
		return Entry{}, err
	}

	entries := db.Entries[host]
	if index < 0 || index >= len(entries) {
		return Entry{}, fmt.Errorf("entry index %d out of range", index)
	}

	removed := entries[index]
	db.Entries[host] = append(entries[:index], entries[index+1:]...)
	if err := SaveHost(dbDir, host, db); err != nil {
		return Entry{}, err
	}

	return removed, nil
}

func appendHostEntry(dbDir, host string, entry Entry) (Entry, error) {
	db, err := LoadHost(dbDir, host)
	if err != nil {
		return Entry{}, err
	}

	db.Entries[host] = append(db.Entries[host], entry)
	if err := SaveHost(dbDir, host, db); err != nil {
		return Entry{}, err
	}

	return entry, nil
}

func isLoggedIn(dbDir, category string) (bool, error) {
	entries, err := LoadAll(dbDir)
	if err != nil {
		return false, err
	}

	status := map[string]bool{}
	for _, entry := range entries {
		cat := normalizeCategory(entry.What)
		switch strings.ToLower(strings.TrimSpace(entry.Action)) {
		case actionLogin:
			status[cat] = true
		case actionLogout:
			status[cat] = false
		}
	}

	return status[category], nil
}

func normalizeCategory(category string) string {
	cat := strings.TrimSpace(category)
	if cat == "" {
		return "work"
	}
	return cat
}

func durationToSeconds(duration time.Duration) int64 {
	return int64(duration / time.Second)
}

func effectiveTime(at time.Time) time.Time {
	if at.IsZero() {
		return time.Now()
	}
	return at
}

func newEntry(action, host, category string, at time.Time, value int64, descr string) Entry {
	eventTime := effectiveTime(at)

	entry := Entry{
		Action: action,
		What:   category,
		Epoch:  eventTime.Unix(),
		Source: host,
		Human:  eventTime.Format("Mon 02.01.2006 15:04:05"),
		Value:  value,
	}
	if trimmedDescr := strings.TrimSpace(descr); trimmedDescr != "" {
		entry.Descr = trimmedDescr
	}

	return entry
}

func normalizeEditedEntry(entry Entry, host string) (Entry, error) {
	action := strings.ToLower(strings.TrimSpace(entry.Action))
	switch action {
	case actionLogin, actionLogout, actionAdd:
	default:
		return Entry{}, fmt.Errorf("unsupported action %q", entry.Action)
	}

	if entry.Epoch <= 0 {
		return Entry{}, errors.New("epoch must be greater than zero")
	}

	entry.Action = action
	entry.What = normalizeCategory(entry.What)
	entry.Source = strings.TrimSpace(entry.Source)
	if entry.Source == "" {
		entry.Source = host
	}
	if entry.Source != host {
		return Entry{}, fmt.Errorf("entry source %q does not match host %q", entry.Source, host)
	}

	if action != actionAdd {
		entry.Value = 0
	}

	if strings.TrimSpace(entry.Human) == "" {
		entry.Human = time.Unix(entry.Epoch, 0).Format("Mon 02.01.2006 15:04:05")
	}
	entry.Descr = strings.TrimSpace(entry.Descr)

	return entry, nil
}
