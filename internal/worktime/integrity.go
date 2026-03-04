package worktime

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// DefaultMaxSessionSpan is the standard maximum allowed login/logout span.
const DefaultMaxSessionSpan = 15 * time.Hour

// IntegrityIssue represents a database consistency violation.
type IntegrityIssue struct {
	Kind     string
	Category string
	Entry    Entry
	Message  string
}

// OpenSession represents a category that is currently logged in.
type OpenSession struct {
	Category string
	Login    Entry
}

// String returns a human-readable description of the issue.
func (i IntegrityIssue) String() string {
	when := time.Unix(i.Entry.Epoch, 0).Format("2006-01-02 15:04:05")
	return fmt.Sprintf("[%s] category=%s source=%s at=%s: %s", i.Kind, i.Category, i.Entry.Source, when, i.Message)
}

// CheckIntegrity loads all databases and validates login/logout consistency.
func CheckIntegrity(dbDir string, maxSessionSpan time.Duration) ([]IntegrityIssue, error) {
	if maxSessionSpan <= 0 {
		return nil, errors.New("max session span must be positive")
	}

	entries, err := LoadAll(dbDir)
	if err != nil {
		return nil, err
	}

	return CheckEntriesIntegrity(entries, maxSessionSpan), nil
}

// CheckEntriesIntegrity validates entry consistency for login/logout flows.
func CheckEntriesIntegrity(entries []Entry, maxSessionSpan time.Duration) []IntegrityIssue {
	activeByCategory := map[string]Entry{}
	issues := make([]IntegrityIssue, 0)

	for _, entry := range entries {
		category := normalizeCategory(entry.What)
		action := strings.ToLower(strings.TrimSpace(entry.Action))

		switch action {
		case actionLogin:
			if previous, active := activeByCategory[category]; active {
				issues = append(issues, IntegrityIssue{
					Kind:     "double-login",
					Category: category,
					Entry:    entry,
					Message: fmt.Sprintf(
						"login while already logged in (previous login at %s from %s)",
						time.Unix(previous.Epoch, 0).Format("2006-01-02 15:04:05"),
						previous.Source,
					),
				})
				continue
			}

			activeByCategory[category] = entry

		case actionLogout:
			login, active := activeByCategory[category]
			if !active {
				issues = append(issues, IntegrityIssue{
					Kind:     "logout-without-login",
					Category: category,
					Entry:    entry,
					Message:  "logout without a matching login",
				})
				continue
			}

			span := time.Duration(entry.Epoch-login.Epoch) * time.Second
			if span <= 0 {
				issues = append(issues, IntegrityIssue{
					Kind:     "non-positive-session",
					Category: category,
					Entry:    entry,
					Message: fmt.Sprintf(
						"logout timestamp is not after login (login at %s)",
						time.Unix(login.Epoch, 0).Format("2006-01-02 15:04:05"),
					),
				})
			}
			if span > maxSessionSpan {
				issues = append(issues, IntegrityIssue{
					Kind:     "session-too-long",
					Category: category,
					Entry:    entry,
					Message:  fmt.Sprintf("session spans %.2fh (max %.2fh)", span.Hours(), maxSessionSpan.Hours()),
				})
			}

			delete(activeByCategory, category)
		}
	}

	return issues
}

// OpenSessions returns categories with an active login that has no corresponding logout yet.
func OpenSessions(entries []Entry) []OpenSession {
	activeByCategory := map[string]Entry{}

	for _, entry := range entries {
		category := normalizeCategory(entry.What)
		action := strings.ToLower(strings.TrimSpace(entry.Action))
		switch action {
		case actionLogin:
			activeByCategory[category] = entry
		case actionLogout:
			delete(activeByCategory, category)
		}
	}

	categories := make([]string, 0, len(activeByCategory))
	for category := range activeByCategory {
		categories = append(categories, category)
	}
	slices.Sort(categories)

	sessions := make([]OpenSession, 0, len(categories))
	for _, category := range categories {
		sessions = append(sessions, OpenSession{
			Category: category,
			Login:    activeByCategory[category],
		})
	}
	return sessions
}
