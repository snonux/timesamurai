// Package config loads sectioned TOML settings for timesamurai.
package config

// Config holds runtime settings loaded from defaults, config.toml, and env.
type Config struct {
	Storage    StorageConfig
	Accounting AccountingConfig
	General    GeneralConfig
	Report     ReportConfig
}

// StorageConfig locates the JSONL store and the legacy worktime JSON directory.
type StorageConfig struct {
	StoreDir string
	DBDir    string
}

// AccountingConfig drives weekly targets and tag categories for the report.
type AccountingConfig struct {
	WeekWorkHours float64
	PlusFor       []string
	MinusFor      []string
	BufferFor     []string
	WeekendDays   []string
}

// GeneralConfig holds host identity and auto-login behaviour.
type GeneralConfig struct {
	Hostname          string
	AutoWorktimeLogin bool
}

// ReportConfig controls report presentation.
type ReportConfig struct {
	Color   bool
	Verbose bool
}

// LoadOptions tune how configuration is loaded at runtime.
type LoadOptions struct {
	// IgnoreEnv skips applying TIMESAMURAI_* environment overrides when true.
	IgnoreEnv bool
	// ConfigPath overrides the global config file path (e.g. via --config).
	ConfigPath string
}

// fileConfig is the TOML decode target. Section tables only; flat keys are rejected.
type fileConfig struct {
	Storage    sectionStorage    `toml:"storage"`
	Accounting sectionAccounting `toml:"accounting"`
	General    sectionGeneral    `toml:"general"`
	Report     sectionReport     `toml:"report"`
}

type sectionStorage struct {
	StoreDir string `toml:"store_dir"`
	DBDir    string `toml:"db_dir"`
}

type sectionAccounting struct {
	WeekWorkHours *float64 `toml:"week_work_hours"`
	PlusFor       []string `toml:"plus_for"`
	MinusFor      []string `toml:"minus_for"`
	BufferFor     []string `toml:"buffer_for"`
	WeekendDays   []string `toml:"weekend_days"`
}

type sectionGeneral struct {
	Hostname          string `toml:"hostname"`
	AutoWorktimeLogin *bool  `toml:"auto_worktime_login"`
}

type sectionReport struct {
	Color   *bool `toml:"color"`
	Verbose *bool `toml:"verbose"`
}

// overlay carries only fields that were explicitly set, for section-wise merge.
type overlay struct {
	Storage    storageOverlay
	Accounting accountingOverlay
	General    generalOverlay
	Report     reportOverlay
}

type storageOverlay struct {
	StoreDir *string
	DBDir    *string
}

type accountingOverlay struct {
	WeekWorkHours *float64
	PlusFor       *[]string
	MinusFor      *[]string
	BufferFor     *[]string
	WeekendDays   *[]string
}

type generalOverlay struct {
	Hostname          *string
	AutoWorktimeLogin *bool
}

type reportOverlay struct {
	Color   *bool
	Verbose *bool
}
