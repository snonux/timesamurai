package config

import (
	"fmt"
	"strings"
)

var validWeekendDays = map[string]struct{}{
	"Mon": {}, "Tue": {}, "Wed": {}, "Thu": {}, "Fri": {}, "Sat": {}, "Sun": {},
}

// Validate reports actionable errors for inconsistent or empty settings.
func (c *Config) Validate() error {
	if err := validateStorage(c.Storage); err != nil {
		return err
	}
	if err := validateAccounting(c.Accounting); err != nil {
		return err
	}
	return nil
}

func validateStorage(s StorageConfig) error {
	if strings.TrimSpace(s.DBDir) == "" {
		return fmt.Errorf("config: storage.db_dir must not be empty")
	}
	if strings.TrimSpace(s.StoreDir) == "" {
		return fmt.Errorf("config: storage.store_dir must not be empty (set it or storage.db_dir)")
	}
	return nil
}

func validateAccounting(a AccountingConfig) error {
	if a.WeekWorkHours <= 0 {
		return fmt.Errorf("config: accounting.week_work_hours must be > 0, got %v", a.WeekWorkHours)
	}
	if err := requireNonEmptyTags("accounting.plus_for", a.PlusFor); err != nil {
		return err
	}
	if err := requireNonEmptyTags("accounting.minus_for", a.MinusFor); err != nil {
		return err
	}
	if err := requireNonEmptyTags("accounting.buffer_for", a.BufferFor); err != nil {
		return err
	}
	if err := requireNonEmptyTags("accounting.weekend_days", a.WeekendDays); err != nil {
		return err
	}
	for _, day := range a.WeekendDays {
		if _, ok := validWeekendDays[day]; !ok {
			return fmt.Errorf("config: accounting.weekend_days contains invalid day %q (use Mon..Sun)", day)
		}
	}
	return nil
}

func requireNonEmptyTags(field string, values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("config: %s must list at least one entry", field)
	}
	for i, v := range values {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("config: %s[%d] must not be empty", field, i)
		}
	}
	return nil
}
