package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// loadOverlayFromEnv builds an overlay from TIMESAMURAI_* environment variables.
// Returns nil, nil when no recognised variables are set. Invalid values error.
func loadOverlayFromEnv() (*overlay, error) {
	out := &overlay{}
	any, err := applyStorageEnv(out)
	if err != nil {
		return nil, err
	}
	a, err := applyAccountingEnv(out)
	if err != nil {
		return nil, err
	}
	any = a || any
	a, err = applyGeneralEnv(out)
	if err != nil {
		return nil, err
	}
	any = a || any
	a, err = applyReportEnv(out)
	if err != nil {
		return nil, err
	}
	any = a || any
	if !any {
		return nil, nil
	}
	return out, nil
}

func applyStorageEnv(out *overlay) (bool, error) {
	any := false
	a, err := applyEnvString(&out.Storage.StoreDir, "TIMESAMURAI_STORE_DIR")
	if err != nil {
		return false, err
	}
	any = a || any
	a, err = applyEnvString(&out.Storage.DBDir, "TIMESAMURAI_DB_DIR")
	if err != nil {
		return false, err
	}
	return a || any, nil
}

func applyAccountingEnv(out *overlay) (bool, error) {
	any := false
	a, err := applyEnvFloat(&out.Accounting.WeekWorkHours, "TIMESAMURAI_WEEK_WORK_HOURS")
	if err != nil {
		return false, err
	}
	any = a || any
	a, err = applyEnvCSV(&out.Accounting.PlusFor, "TIMESAMURAI_PLUS_FOR")
	if err != nil {
		return false, err
	}
	any = a || any
	a, err = applyEnvCSV(&out.Accounting.MinusFor, "TIMESAMURAI_MINUS_FOR")
	if err != nil {
		return false, err
	}
	any = a || any
	a, err = applyEnvCSV(&out.Accounting.BufferFor, "TIMESAMURAI_BUFFER_FOR")
	if err != nil {
		return false, err
	}
	any = a || any
	a, err = applyEnvCSV(&out.Accounting.WeekendDays, "TIMESAMURAI_WEEKEND_DAYS")
	if err != nil {
		return false, err
	}
	return a || any, nil
}

func applyGeneralEnv(out *overlay) (bool, error) {
	any := false
	a, err := applyEnvString(&out.General.Hostname, "TIMESAMURAI_HOSTNAME")
	if err != nil {
		return false, err
	}
	any = a || any
	a, err = applyEnvBool(&out.General.AutoWorktimeLogin, "TIMESAMURAI_AUTO_WORKTIME_LOGIN")
	if err != nil {
		return false, err
	}
	return a || any, nil
}

func applyReportEnv(out *overlay) (bool, error) {
	any := false
	a, err := applyEnvBool(&out.Report.Color, "TIMESAMURAI_COLOR")
	if err != nil {
		return false, err
	}
	any = a || any
	a, err = applyEnvBool(&out.Report.Verbose, "TIMESAMURAI_VERBOSE")
	if err != nil {
		return false, err
	}
	return a || any, nil
}

func applyEnvString(target **string, key string) (bool, error) {
	value := getenvTrim(key)
	if value == "" {
		return false, nil
	}
	*target = &value
	return true, nil
}

func applyEnvFloat(target **float64, key string) (bool, error) {
	value := getenvTrim(key)
	if value == "" {
		return false, nil
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return false, fmt.Errorf("env %s: invalid float %q: %w", key, value, err)
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return false, fmt.Errorf("env %s: non-finite float %q", key, value)
	}
	*target = &f
	return true, nil
}

func applyEnvCSV(target **[]string, key string) (bool, error) {
	value := getenvTrim(key)
	if value == "" {
		return false, nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	*target = &out
	return true, nil
}

func applyEnvBool(target **bool, key string) (bool, error) {
	value := getenvTrim(key)
	if value == "" {
		return false, nil
	}
	parsed, err := parseEnvBool(value)
	if err != nil {
		return false, fmt.Errorf("env %s: %w", key, err)
	}
	*target = &parsed
	return true, nil
}

func parseEnvBool(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool %q (want true/false/yes/no/1/0/on/off)", value)
	}
}

func getenvTrim(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}
