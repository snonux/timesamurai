package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Load reads configuration from the TOML file (if present), merges TIMESAMURAI_*
// environment overrides, validates, and expands ~ in path fields.
// A missing config file is not an error; defaults are used.
func Load(ctx context.Context, opts LoadOptions) (Config, error) {
	cfg := Default()
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return cfg, fmt.Errorf("config load cancelled: %w", err)
		}
	}

	path, err := resolveConfigPath(opts.ConfigPath)
	if err != nil {
		return cfg, err
	}

	fileOverlay, err := loadOverlayFromFile(path)
	if err != nil {
		return cfg, err
	}
	if fileOverlay != nil {
		cfg.mergeWith(fileOverlay)
		applyStoreDirFallback(&cfg, fileOverlay)
	}

	if !opts.IgnoreEnv {
		envOverlay, err := loadOverlayFromEnv()
		if err != nil {
			return cfg, err
		}
		if envOverlay != nil {
			cfg.mergeWith(envOverlay)
			applyStoreDirFallback(&cfg, envOverlay)
		}
	}

	if err := cfg.normalize(); err != nil {
		return cfg, err
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// applyStoreDirFallback clears store_dir when an overlay sets db_dir but not
// store_dir, so normalize can copy db_dir into store_dir ("defaults to db_dir
// when unset").
func applyStoreDirFallback(cfg *Config, ov *overlay) {
	if ov.Storage.DBDir != nil && ov.Storage.StoreDir == nil {
		cfg.Storage.StoreDir = ""
	}
}

// ConfigPath returns $XDG_CONFIG_HOME/timesamurai/config.toml, falling back to
// ~/.config/timesamurai/config.toml when XDG_CONFIG_HOME is unset.
func ConfigPath() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, configDirName, configFileName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".config", configDirName, configFileName), nil
}

func resolveConfigPath(override string) (string, error) {
	if strings.TrimSpace(override) == "" {
		return ConfigPath()
	}
	expanded, err := expandHome(override)
	if err != nil {
		return "", fmt.Errorf("expand config path %q: %w", override, err)
	}
	return expanded, nil
}

// loadOverlayFromFile returns nil, nil when the file does not exist.
func loadOverlayFromFile(path string) (*overlay, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	tables, raw, err := decodeTOML(data)
	if err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	if err := rejectUnknownKeys(raw); err != nil {
		return nil, fmt.Errorf("config %q: %w", path, err)
	}
	return tables.toOverlay(), nil
}

func decodeTOML(data []byte) (*fileConfig, map[string]any, error) {
	var tables fileConfig
	if err := toml.Unmarshal(data, &tables); err != nil {
		return nil, nil, err
	}
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, nil, err
	}
	return &tables, raw, nil
}

func rejectUnknownKeys(raw map[string]any) error {
	knownTop := map[string]struct{}{
		"storage": {}, "accounting": {}, "general": {}, "report": {},
	}
	knownSections := map[string]map[string]struct{}{
		"storage": {
			"store_dir": {}, "db_dir": {},
		},
		"accounting": {
			"week_work_hours": {}, "plus_for": {}, "minus_for": {},
			"buffer_for": {}, "weekend_days": {},
		},
		"general": {
			"hostname": {}, "auto_worktime_login": {},
		},
		"report": {
			"color": {}, "verbose": {},
		},
	}

	for key, value := range raw {
		if _, ok := knownTop[key]; !ok {
			return fmt.Errorf("unsupported top-level key %q; use sectioned tables [storage], [accounting], [general], [report]", key)
		}
		section, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("top-level key %q must be a table", key)
		}
		allowed := knownSections[key]
		for nested := range section {
			if _, ok := allowed[nested]; !ok {
				return fmt.Errorf("unsupported key %q.%q", key, nested)
			}
		}
	}
	return nil
}

func (fc *fileConfig) toOverlay() *overlay {
	out := &overlay{}
	applyStorageSection(fc, out)
	applyAccountingSection(fc, out)
	applyGeneralSection(fc, out)
	applyReportSection(fc, out)
	return out
}

func applyStorageSection(fc *fileConfig, out *overlay) {
	if s := strings.TrimSpace(fc.Storage.StoreDir); s != "" {
		out.Storage.StoreDir = stringPtr(s)
	}
	if s := strings.TrimSpace(fc.Storage.DBDir); s != "" {
		out.Storage.DBDir = stringPtr(s)
	}
}

func applyAccountingSection(fc *fileConfig, out *overlay) {
	if fc.Accounting.WeekWorkHours != nil {
		v := *fc.Accounting.WeekWorkHours
		out.Accounting.WeekWorkHours = &v
	}
	if fc.Accounting.PlusFor != nil {
		s := slicesClone(fc.Accounting.PlusFor)
		out.Accounting.PlusFor = &s
	}
	if fc.Accounting.MinusFor != nil {
		s := slicesClone(fc.Accounting.MinusFor)
		out.Accounting.MinusFor = &s
	}
	if fc.Accounting.BufferFor != nil {
		s := slicesClone(fc.Accounting.BufferFor)
		out.Accounting.BufferFor = &s
	}
	if fc.Accounting.WeekendDays != nil {
		s := slicesClone(fc.Accounting.WeekendDays)
		out.Accounting.WeekendDays = &s
	}
}

func applyGeneralSection(fc *fileConfig, out *overlay) {
	if s := strings.TrimSpace(fc.General.Hostname); s != "" {
		out.General.Hostname = stringPtr(s)
	}
	if fc.General.AutoWorktimeLogin != nil {
		out.General.AutoWorktimeLogin = fc.General.AutoWorktimeLogin
	}
}

func applyReportSection(fc *fileConfig, out *overlay) {
	if fc.Report.Color != nil {
		out.Report.Color = fc.Report.Color
	}
	if fc.Report.Verbose != nil {
		out.Report.Verbose = fc.Report.Verbose
	}
}

func (c *Config) normalize() error {
	if strings.TrimSpace(c.Storage.StoreDir) == "" {
		c.Storage.StoreDir = c.Storage.DBDir
	}
	dbDir, err := expandHome(c.Storage.DBDir)
	if err != nil {
		return fmt.Errorf("expand storage.db_dir %q: %w", c.Storage.DBDir, err)
	}
	storeDir, err := expandHome(c.Storage.StoreDir)
	if err != nil {
		return fmt.Errorf("expand storage.store_dir %q: %w", c.Storage.StoreDir, err)
	}
	c.Storage.DBDir = dbDir
	c.Storage.StoreDir = storeDir
	return nil
}

func expandHome(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

func stringPtr(s string) *string { return &s }

func slicesClone(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
