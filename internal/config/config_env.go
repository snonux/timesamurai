package config

import (
	"os"
	"strconv"
	"strings"
)

// loadOverlayFromEnv builds an overlay from TIMESAMURAI_* environment variables.
// Returns nil when no recognised variables are set.
func loadOverlayFromEnv() *overlay {
	out := &overlay{}
	any := applyStorageEnv(out)
	any = applyAccountingEnv(out) || any
	any = applyGeneralEnv(out) || any
	any = applyReportEnv(out) || any
	if !any {
		return nil
	}
	return out
}

func applyStorageEnv(out *overlay) bool {
	any := false
	any = applyEnvString(&out.Storage.StoreDir, "TIMESAMURAI_STORE_DIR") || any
	any = applyEnvString(&out.Storage.DBDir, "TIMESAMURAI_DB_DIR") || any
	return any
}

func applyAccountingEnv(out *overlay) bool {
	any := false
	any = applyEnvFloat(&out.Accounting.WeekWorkHours, "TIMESAMURAI_WEEK_WORK_HOURS") || any
	any = applyEnvCSV(&out.Accounting.PlusFor, "TIMESAMURAI_PLUS_FOR") || any
	any = applyEnvCSV(&out.Accounting.MinusFor, "TIMESAMURAI_MINUS_FOR") || any
	any = applyEnvCSV(&out.Accounting.BufferFor, "TIMESAMURAI_BUFFER_FOR") || any
	any = applyEnvCSV(&out.Accounting.WeekendDays, "TIMESAMURAI_WEEKEND_DAYS") || any
	return any
}

func applyGeneralEnv(out *overlay) bool {
	any := false
	any = applyEnvString(&out.General.Hostname, "TIMESAMURAI_HOSTNAME") || any
	any = applyEnvBool(&out.General.AutoWorktimeLogin, "TIMESAMURAI_AUTO_WORKTIME_LOGIN") || any
	return any
}

func applyReportEnv(out *overlay) bool {
	any := false
	any = applyEnvBool(&out.Report.Color, "TIMESAMURAI_COLOR") || any
	any = applyEnvBool(&out.Report.Verbose, "TIMESAMURAI_VERBOSE") || any
	return any
}

func applyEnvString(target **string, key string) bool {
	value := getenvTrim(key)
	if value == "" {
		return false
	}
	*target = &value
	return true
}

func applyEnvFloat(target **float64, key string) bool {
	value := getenvTrim(key)
	if value == "" {
		return false
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return false
	}
	*target = &f
	return true
}

func applyEnvCSV(target **[]string, key string) bool {
	value := getenvTrim(key)
	if value == "" {
		return false
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	*target = &out
	return true
}

func applyEnvBool(target **bool, key string) bool {
	value := getenvTrim(key)
	if value == "" {
		return false
	}
	parsed := value == "true" || value == "1" || strings.EqualFold(value, "yes")
	*target = &parsed
	return true
}

func getenvTrim(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}
