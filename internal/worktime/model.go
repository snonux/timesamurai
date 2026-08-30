package worktime

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/snonux/timesamurai/internal/config"
)

const (
	// WorkTag is the default accounting category for tracked time.
	WorkTag = "work"

	actionLogin  = "login"
	actionLogout = "logout"
	actionAdd    = "add"
)

// ErrMultipleAccountingTags indicates more than one report accounting tag on an entry.
var ErrMultipleAccountingTags = errors.New("multiple accounting tags")

// Entry is one JSONL work-time event for a host.
// JSON field order matches the on-disk plan examples for stable diffs:
// id, action, epoch, host, value, tags, descr.
type Entry struct {
	ID     int64    `json:"id"`
	Action string   `json:"action"`
	Epoch  int64    `json:"epoch"`
	Host   string   `json:"host"`
	Value  int64    `json:"value,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	Descr  string   `json:"descr,omitempty"`
}

// TagClass describes how a tag maps to accounting configuration lists.
type TagClass int

const (
	TagClassLabel TagClass = iota
	TagClassWork
	TagClassPlus
	TagClassMinus
	TagClassBuffer
)

// ClassifyTag maps tag against cfg accounting lists and WorkTag.
func ClassifyTag(cfg config.AccountingConfig, tag string) TagClass {
	normalized := strings.TrimSpace(tag)
	if normalized == "" {
		return TagClassLabel
	}
	if normalized == WorkTag {
		return TagClassWork
	}
	if slices.Contains(cfg.PlusFor, normalized) {
		return TagClassPlus
	}
	if slices.Contains(cfg.MinusFor, normalized) {
		return TagClassMinus
	}
	if slices.Contains(cfg.BufferFor, normalized) {
		return TagClassBuffer
	}
	return TagClassLabel
}

// AccountingTag returns the single report accounting tag for tags.
//
// The first work/plus/minus tag becomes the accounting tag; further tags from
// those lists are rejected. When no primary tag is present, the first buffer
// tag accounts. Additional buffer tags are labels once a primary tag exists,
// but a second buffer tag is rejected when buffer alone would account.
func AccountingTag(cfg config.AccountingConfig, tags []string) (string, error) {
	normalized, err := normalizeTags(tags)
	if err != nil {
		return "", err
	}

	var accounting string
	var primarySet bool

	for _, tag := range normalized {
		switch ClassifyTag(cfg, tag) {
		case TagClassWork, TagClassPlus, TagClassMinus:
			if accounting != "" {
				return "", fmt.Errorf("%w: %q and %q", ErrMultipleAccountingTags, accounting, tag)
			}
			accounting = tag
			primarySet = true
		case TagClassBuffer:
			if accounting == "" {
				accounting = tag
				continue
			}
			if primarySet {
				continue
			}
			return "", fmt.Errorf("%w: %q and %q", ErrMultipleAccountingTags, accounting, tag)
		}
	}

	return accounting, nil
}

// ValidateTags ensures tags obey the single accounting-tag rule.
func ValidateTags(cfg config.AccountingConfig, tags []string) error {
	_, err := AccountingTag(cfg, tags)
	return err
}

// isValidAction reports whether action (already lowercased/trimmed by the
// caller) is one of the three actions the store and reporter understand.
// Shared by ValidateEntry (live inserts/patches, via insertEntry/patchEntry)
// and by migrate's legacy import, so a row that BuildReport would reject
// with "unknown action" is caught at the same place for both entry points
// instead of only surfacing later when a report is built (task 781).
func isValidAction(action string) bool {
	switch action {
	case actionLogin, actionLogout, actionAdd:
		return true
	default:
		return false
	}
}

// ValidateEntry checks required fields and tag accounting rules.
func ValidateEntry(cfg config.AccountingConfig, entry Entry) error {
	if entry.ID <= 0 {
		return errors.New("entry id must be positive")
	}

	host := strings.TrimSpace(entry.Host)
	if host == "" {
		return errors.New("entry host must not be empty")
	}

	action := strings.ToLower(strings.TrimSpace(entry.Action))
	if !isValidAction(action) {
		return fmt.Errorf("unsupported action %q", entry.Action)
	}

	if entry.Epoch <= 0 {
		return errors.New("entry epoch must be positive")
	}

	if action != actionAdd && entry.Value != 0 {
		return fmt.Errorf("action %q must not carry value", action)
	}

	if err := ValidateTags(cfg, entry.Tags); err != nil {
		return err
	}

	return nil
}

func normalizeTags(tags []string) ([]string, error) {
	if len(tags) == 0 {
		return nil, nil
	}

	normalized := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			return nil, errors.New("tag must not be empty")
		}
		if _, ok := seen[trimmed]; ok {
			return nil, fmt.Errorf("duplicate tag %q", trimmed)
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized, nil
}
