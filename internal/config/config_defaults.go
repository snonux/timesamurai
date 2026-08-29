package config

import "slices"

const (
	configDirName           = "timesamurai"
	configFileName          = "config.toml"
	configPathFallback      = "$XDG_CONFIG_HOME/timesamurai/config.toml"
	defaultWeekWorkHours    = 40.0
	defaultWorktimeDBDir    = "~/git/worktime"
	defaultWorktimeStoreDir = "~/git/worktime/timesamuraidb"
)

var (
	defaultPlusFor   = []string{"off", "bank", "bufferuse", "sick"}
	defaultMinusFor  = []string{"lunch"}
	defaultBufferFor = []string{
		"tools",
		"pet",
		"selfdevelopment",
		"workrebalance",
		"compensate",
		"travel",
		"rebalance",
	}
	defaultWeekendDays = []string{"Sat", "Sun"}
)

// Default returns built-in configuration values.
func Default() Config {
	return Config{
		Storage: StorageConfig{
			StoreDir: defaultWorktimeStoreDir,
			DBDir:    defaultWorktimeDBDir,
		},
		Accounting: AccountingConfig{
			WeekWorkHours: defaultWeekWorkHours,
			PlusFor:       slices.Clone(defaultPlusFor),
			MinusFor:      slices.Clone(defaultMinusFor),
			BufferFor:     slices.Clone(defaultBufferFor),
			WeekendDays:   slices.Clone(defaultWeekendDays),
		},
		General: GeneralConfig{
			Hostname:          "",
			AutoWorktimeLogin: false,
		},
		Report: ReportConfig{
			Color:   true,
			Verbose: false,
		},
	}
}

// DefaultConfigPath returns the resolved config path, or the documented XDG
// fallback string when the real path cannot be determined.
func DefaultConfigPath() string {
	path, err := ConfigPath()
	if err != nil {
		return configPathFallback
	}
	return path
}
