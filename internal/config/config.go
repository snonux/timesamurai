package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultWeekWorkHours = 40.0
	defaultWorktimeDBDir = "~/git/worktime"
	configDirName        = "timesamurai"
	configFileName       = "config.json"
)

var (
	defaultPlusFor     = []string{"off", "bank", "bufferuse", "sick"}
	defaultWeekendDays = []string{"Sat", "Sun"}
	defaultMinusFor    = []string{"lunch"}
	defaultBufferFor   = []string{
		"tools",
		"pet",
		"selfdevelopment",
		"workrebalance",
		"compensate",
		"travel",
		"rebalance",
	}
)

// Config defines runtime settings for timer and worktime integrations.
type Config struct {
	WeekWorkHours     float64  `json:"weekworkhours"`
	PlusFor           []string `json:"plusfor"`
	WeekendDays       []string `json:"weekendays"`
	MinusFor          []string `json:"minusfor"`
	BufferFor         []string `json:"bufferfor"`
	WorktimeDBDir     string   `json:"worktime_db_dir"`
	Hostname          string   `json:"hostname"`
	AutoWorktimeLogin bool     `json:"auto_worktime_login"`
}

// Default returns the default configuration values.
func Default() Config {
	return Config{
		WeekWorkHours:     defaultWeekWorkHours,
		PlusFor:           cloneStrings(defaultPlusFor),
		WeekendDays:       cloneStrings(defaultWeekendDays),
		MinusFor:          cloneStrings(defaultMinusFor),
		BufferFor:         cloneStrings(defaultBufferFor),
		WorktimeDBDir:     defaultWorktimeDBDir,
		Hostname:          "",
		AutoWorktimeLogin: false,
	}
}

// DefaultPath returns the default config file location.
func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}

	return filepath.Join(configDir, configDirName, configFileName), nil
}

// Load reads config from path. If path is empty, the default path is used.
// Missing config files return defaults.
func Load(path string) (Config, error) {
	cfg := Default()

	configPath, err := resolveConfigPath(path)
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return normalizeConfig(cfg)
		}
		return cfg, fmt.Errorf("read config %q: %w", configPath, err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %q: %w", configPath, err)
	}

	applyDefaults(&cfg)
	return normalizeConfig(cfg)
}

// Save writes config to path. If path is empty, the default path is used.
func Save(path string, cfg Config) error {
	configPath, err := resolveConfigPath(path)
	if err != nil {
		return err
	}

	applyDefaults(&cfg)
	cfg, err = normalizeConfig(cfg)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create config directory for %q: %w", configPath, err)
	}

	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("write config %q: %w", configPath, err)
	}

	return nil
}

// EffectiveHostname resolves hostname using config, then ~/.hostnameoverride, then os.Hostname().
func (c Config) EffectiveHostname() (string, error) {
	if host := strings.TrimSpace(c.Hostname); host != "" {
		return host, nil
	}

	overridePath, err := expandHome("~/.hostnameoverride")
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(overridePath)
	if err == nil {
		if host := strings.TrimSpace(string(data)); host != "" {
			return host, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read hostname override %q: %w", overridePath, err)
	}

	host, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("resolve os hostname: %w", err)
	}

	return host, nil
}

func applyDefaults(cfg *Config) {
	if cfg.WeekWorkHours == 0 {
		cfg.WeekWorkHours = defaultWeekWorkHours
	}
	if cfg.PlusFor == nil {
		cfg.PlusFor = cloneStrings(defaultPlusFor)
	}
	if cfg.WeekendDays == nil {
		cfg.WeekendDays = cloneStrings(defaultWeekendDays)
	}
	if cfg.MinusFor == nil {
		cfg.MinusFor = cloneStrings(defaultMinusFor)
	}
	if cfg.BufferFor == nil {
		cfg.BufferFor = cloneStrings(defaultBufferFor)
	}
	if strings.TrimSpace(cfg.WorktimeDBDir) == "" {
		cfg.WorktimeDBDir = defaultWorktimeDBDir
	}
}

func normalizeConfig(cfg Config) (Config, error) {
	worktimeDir, err := expandHome(cfg.WorktimeDBDir)
	if err != nil {
		return cfg, fmt.Errorf("expand worktime_db_dir %q: %w", cfg.WorktimeDBDir, err)
	}
	cfg.WorktimeDBDir = worktimeDir
	return cfg, nil
}

func resolveConfigPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return DefaultPath()
	}

	resolvedPath, err := expandHome(path)
	if err != nil {
		return "", fmt.Errorf("expand config path %q: %w", path, err)
	}
	return resolvedPath, nil
}

func expandHome(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	if path == "~" {
		return homeDir, nil
	}

	return filepath.Join(homeDir, path[2:]), nil
}

func cloneStrings(values []string) []string {
	copied := make([]string, len(values))
	copy(copied, values)
	return copied
}
