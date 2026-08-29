package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const (
	legacyConfigFileName = "config.json"
	migrateHeader        = "# Migrated from config.json by timesamurai. Legacy JSON left in place.\n"
)

// legacyJSON is the flat pre-rewrite config.json shape (pre-rewrite tag).
type legacyJSON struct {
	WeekWorkHours     *float64 `json:"weekworkhours"`
	PlusFor           []string `json:"plusfor"`
	WeekendDays       []string `json:"weekendays"`
	MinusFor          []string `json:"minusfor"`
	BufferFor         []string `json:"bufferfor"`
	WorktimeDBDir     *string  `json:"worktime_db_dir"`
	Hostname          *string  `json:"hostname"`
	AutoWorktimeLogin *bool    `json:"auto_worktime_login"`
}

// migrateDoc is the TOML encode target for a one-shot JSON→TOML conversion.
type migrateDoc struct {
	Storage    *migrateStorage    `toml:"storage,omitempty"`
	Accounting *migrateAccounting `toml:"accounting,omitempty"`
	General    *migrateGeneral    `toml:"general,omitempty"`
}

type migrateStorage struct {
	StoreDir string `toml:"store_dir,omitempty"`
	DBDir    string `toml:"db_dir,omitempty"`
}

type migrateAccounting struct {
	WeekWorkHours *float64  `toml:"week_work_hours,omitempty"`
	PlusFor       *[]string `toml:"plus_for,omitempty"`
	MinusFor      *[]string `toml:"minus_for,omitempty"`
	BufferFor     *[]string `toml:"buffer_for,omitempty"`
	WeekendDays   *[]string `toml:"weekend_days,omitempty"`
}

type migrateGeneral struct {
	Hostname          string `toml:"hostname,omitempty"`
	AutoWorktimeLogin *bool  `toml:"auto_worktime_login,omitempty"`
}

// maybeMigrateLegacyJSON converts config.json → config.toml once when TOML is
// missing. When both exist, TOML wins and a one-line notice is written. The
// JSON file is never deleted.
func maybeMigrateLegacyJSON(ctx context.Context, tomlPath string, notice io.Writer) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("config migrate cancelled: %w", err)
		}
	}

	jsonPath := legacyJSONPath(tomlPath)
	jsonExists, err := pathExists(jsonPath)
	if err != nil {
		return err
	}
	if !jsonExists {
		return nil
	}

	tomlExists, err := pathExists(tomlPath)
	if err != nil {
		return err
	}
	if tomlExists {
		writeJSONIgnoredNotice(notice, jsonPath, tomlPath)
		return nil
	}

	return migrateJSONToTOML(ctx, jsonPath, tomlPath)
}

func writeJSONIgnoredNotice(notice io.Writer, jsonPath, tomlPath string) {
	if notice == nil {
		return
	}
	// Intentional: remind on every Load while both files remain (spec: TOML wins,
	// leave JSON in place until the user removes it).
	_, _ = fmt.Fprintf(notice, "timesamurai: ignoring legacy %s; using %s\n", jsonPath, tomlPath)
}

func migrateJSONToTOML(ctx context.Context, jsonPath, tomlPath string) error {
	_ = ctx // cancellation already checked by maybeMigrateLegacyJSON

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("read legacy config %q: %w", jsonPath, err)
	}

	var leg legacyJSON
	if err := json.Unmarshal(data, &leg); err != nil {
		return fmt.Errorf("parse legacy config %q: %w", jsonPath, err)
	}

	encoded, err := toml.Marshal(buildMigrateDoc(leg))
	if err != nil {
		return fmt.Errorf("encode migrated config: %w", err)
	}

	dir := filepath.Dir(tomlPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config directory for %q: %w", tomlPath, err)
	}

	out := append([]byte(migrateHeader), encoded...)
	tmp, err := os.CreateTemp(dir, "config.toml.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp migrated config: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp migrated config %q: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp migrated config %q: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, tomlPath); err != nil {
		return fmt.Errorf("install migrated config %q: %w", tomlPath, err)
	}
	return nil
}

func buildMigrateDoc(leg legacyJSON) migrateDoc {
	doc := migrateDoc{}
	doc.Storage = buildMigrateStorage(leg)
	doc.Accounting = buildMigrateAccounting(leg)
	doc.General = buildMigrateGeneral(leg)
	return doc
}

func buildMigrateStorage(leg legacyJSON) *migrateStorage {
	s := &migrateStorage{StoreDir: defaultWorktimeStoreDir}
	if leg.WorktimeDBDir != nil {
		if d := *leg.WorktimeDBDir; d != "" {
			s.DBDir = d
		}
	}
	return s
}

func buildMigrateAccounting(leg legacyJSON) *migrateAccounting {
	a := &migrateAccounting{}
	any := false
	if leg.WeekWorkHours != nil && *leg.WeekWorkHours != 0 {
		v := *leg.WeekWorkHours
		a.WeekWorkHours = &v
		any = true
	}
	if leg.PlusFor != nil {
		s := slicesClone(leg.PlusFor)
		a.PlusFor = &s
		any = true
	}
	if leg.MinusFor != nil {
		s := slicesClone(leg.MinusFor)
		a.MinusFor = &s
		any = true
	}
	if leg.BufferFor != nil {
		s := slicesClone(leg.BufferFor)
		a.BufferFor = &s
		any = true
	}
	if leg.WeekendDays != nil {
		s := slicesClone(leg.WeekendDays)
		a.WeekendDays = &s
		any = true
	}
	if !any {
		return nil
	}
	return a
}

func buildMigrateGeneral(leg legacyJSON) *migrateGeneral {
	g := &migrateGeneral{}
	any := false
	if leg.Hostname != nil {
		if h := *leg.Hostname; h != "" {
			g.Hostname = h
			any = true
		}
	}
	if leg.AutoWorktimeLogin != nil {
		g.AutoWorktimeLogin = leg.AutoWorktimeLogin
		any = true
	}
	if !any {
		return nil
	}
	return g
}

func legacyJSONPath(tomlPath string) string {
	return filepath.Join(filepath.Dir(tomlPath), legacyConfigFileName)
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat %q: %w", path, err)
}
