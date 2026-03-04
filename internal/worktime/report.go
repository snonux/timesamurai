package worktime

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"codeberg.org/snonux/timesamurai/internal/config"
)

const secondsPerHour = int64(3600)

const (
	colorReset = "\033[0m"
	colorCyan  = "\033[36m"
	colorGreen = "\033[32m"
	colorRed   = "\033[31m"
)

// DayReport contains report data for one calendar day.
type DayReport struct {
	Epoch           int64
	DayLabel        string
	Marker          string
	Values          map[string]int64
	RequiredSeconds int64
}

// WeekReport contains report data for one ISO week.
type WeekReport struct {
	WeekLabel                string
	Days                     []DayReport
	Values                   map[string]int64
	RequiredSeconds          int64
	WeeklyBalanceSeconds     int64
	CumulativeBalanceSeconds int64
	BufferSeconds            int64
}

type dayAccumulator struct {
	epoch  int64
	values map[string]int64
}

type weekAccumulator struct {
	weekLabel string
	days      []DayReport
	values    map[string]int64
}

type reportBuildContext struct {
	login             map[string]Entry
	totalBuffer       int64
	cumulativeBalance int64
	reports           []WeekReport
	currentDay        dayAccumulator
	currentWeek       weekAccumulator
	prevDayKey        string
	prevWeekKey       string
}

// BuildReport generates weekly reports from merged worktime entries.
func BuildReport(entries []Entry, cfg config.Config) ([]WeekReport, error) {
	if len(entries) == 0 {
		return []WeekReport{}, nil
	}

	cfg = reportDefaults(cfg)
	sorted := make([]Entry, len(entries))
	copy(sorted, entries)
	sortEntries(sorted)

	bufferFor := stringSet(cfg.BufferFor)
	minusFor := stringSet(cfg.MinusFor)
	weekendDays := stringSet(cfg.WeekendDays)
	plusFor := stringSet(cfg.PlusFor)

	ctx := reportBuildContext{
		login:       map[string]Entry{},
		currentDay:  newDayAccumulator(),
		currentWeek: newWeekAccumulator(),
	}

	for _, entry := range sorted {
		if err := processEntry(&ctx, entry, cfg, plusFor, minusFor, weekendDays, bufferFor); err != nil {
			return nil, err
		}
	}

	finalizeDayIntoWeek(&ctx.currentWeek, ctx.currentDay, minusFor, weekendDays)
	weekReport := finalizeWeek(ctx.currentWeek, cfg, plusFor, minusFor, ctx.totalBuffer, &ctx.cumulativeBalance)
	ctx.reports = append(ctx.reports, weekReport)

	return ctx.reports, nil
}

func processEntry(
	ctx *reportBuildContext,
	entry Entry,
	cfg config.Config,
	plusFor map[string]struct{},
	minusFor map[string]struct{},
	weekendDays map[string]struct{},
	bufferFor map[string]struct{},
) error {
	entryDayKey := dayKey(entry.Epoch)
	entryWeekKey := isoWeekKey(entry.Epoch)

	if ctx.prevDayKey == "" {
		ctx.prevDayKey = entryDayKey
	}
	if ctx.prevWeekKey == "" {
		ctx.prevWeekKey = entryWeekKey
		ctx.currentWeek.weekLabel = weekLabel(entry.Epoch)
	}

	if entryDayKey != ctx.prevDayKey {
		finalizeDayIntoWeek(&ctx.currentWeek, ctx.currentDay, minusFor, weekendDays)
		ctx.currentDay = newDayAccumulator()
		ctx.prevDayKey = entryDayKey
	}

	if entryWeekKey != ctx.prevWeekKey {
		weekReport := finalizeWeek(ctx.currentWeek, cfg, plusFor, minusFor, ctx.totalBuffer, &ctx.cumulativeBalance)
		ctx.reports = append(ctx.reports, weekReport)
		ctx.currentWeek = newWeekAccumulator()
		ctx.currentWeek.weekLabel = weekLabel(entry.Epoch)
		ctx.prevWeekKey = entryWeekKey
	}

	category := normalizeCategory(entry.What)
	if ctx.currentDay.epoch == 0 {
		ctx.currentDay.epoch = entry.Epoch
	}
	if _, ok := ctx.currentDay.values[category]; !ok {
		ctx.currentDay.values[category] = 0
	}

	action := strings.ToLower(strings.TrimSpace(entry.Action))
	switch action {
	case actionAdd:
		ctx.currentDay.values[category] += entry.Value
		if _, ok := bufferFor[category]; ok {
			ctx.totalBuffer += entry.Value
		}
	case actionLogin:
		if _, ok := ctx.login[category]; ok {
			return fmt.Errorf("already logged in for %q at epoch %d", category, entry.Epoch)
		}
		ctx.login[category] = entry
	case actionLogout:
		startEntry, ok := ctx.login[category]
		if !ok {
			return fmt.Errorf("logout without login for %q at epoch %d", category, entry.Epoch)
		}
		ctx.currentDay.values[category] += entry.Epoch - startEntry.Epoch
		delete(ctx.login, category)
	default:
		return fmt.Errorf("unknown action %q at epoch %d", entry.Action, entry.Epoch)
	}

	return nil
}

// FormatReport renders week/day reports as text. Colors can be toggled.
func FormatReport(weeks []WeekReport, verbose, color bool) string {
	var out strings.Builder

	for weekIdx := range weeks {
		week := &weeks[weekIdx]
		for dayIdx := range week.Days {
			day := &week.Days[dayIdx]
			out.WriteString("   ")
			out.WriteString(day.Marker)
			out.WriteString(" ")
			out.WriteString(day.DayLabel)
			out.WriteString(":")
			out.WriteString(formatData(day.Values, 0, day.Epoch, verbose, color))
			out.WriteString("\n")
		}

		out.WriteString("================================================\n")
		weekValues := cloneValueMap(week.Values)
		weekValues["balance"] = week.CumulativeBalanceSeconds
		out.WriteString(formatData(weekValues, week.BufferSeconds, 0, false, color))
		out.WriteString("\n\n\n")
	}

	return out.String()
}

func finalizeDayIntoWeek(week *weekAccumulator, day dayAccumulator, minusFor map[string]struct{}, weekendDays map[string]struct{}) {
	if day.epoch == 0 {
		return
	}

	for key, value := range day.values {
		week.values[key] += value
	}

	dayValues := cloneValueMap(day.values)
	for category := range minusFor {
		if value, ok := dayValues[category]; ok {
			dayValues["work"] -= value
		}
	}

	marker := dayMarker(day.epoch, day.values, weekendDays)
	required := int64(8) * secondsPerHour
	if marker == "*" {
		required = 0
	}

	week.days = append(week.days, DayReport{
		Epoch:           day.epoch,
		DayLabel:        dayLabel(day.epoch),
		Marker:          marker,
		Values:          dayValues,
		RequiredSeconds: required,
	})
}

func finalizeWeek(
	week weekAccumulator,
	cfg config.Config,
	plusFor map[string]struct{},
	minusFor map[string]struct{},
	totalBuffer int64,
	cumulativeBalance *int64,
) WeekReport {
	values := cloneValueMap(week.values)

	required := int64(math.Round(cfg.WeekWorkHours * float64(secondsPerHour)))
	for category := range plusFor {
		required -= values[category]
	}

	work := values["work"]
	for category := range minusFor {
		work -= values[category]
	}
	values["work"] = work

	weeklyBalance := work - required
	*cumulativeBalance += weeklyBalance

	return WeekReport{
		WeekLabel:                week.weekLabel,
		Days:                     week.days,
		Values:                   values,
		RequiredSeconds:          required,
		WeeklyBalanceSeconds:     weeklyBalance,
		CumulativeBalanceSeconds: *cumulativeBalance,
		BufferSeconds:            totalBuffer,
	}
}

func formatData(values map[string]int64, bufferSeconds int64, epoch int64, verbose, color bool) string {
	keys := []string{"balance", "work", "lunch", "off", "sick", "bank", "pet", "selfdevelopment"}
	var out strings.Builder

	for _, key := range keys {
		value, hasValue := values[key]
		if !hasValue && key != "work" {
			continue
		}
		if !hasValue {
			value = 0
		}
		if value == 0 && key != "work" {
			continue
		}

		out.WriteString(" ")
		out.WriteString(colorizeLabel(key, color))
		out.WriteString(":")
		out.WriteString(colorizeValue(formatHours(value), value, color))
		out.WriteString("h")
	}

	if bufferSeconds != 0 {
		out.WriteString(" ")
		out.WriteString(colorizeLabel("buffer", color))
		out.WriteString(":")
		out.WriteString(colorizeValue(formatHours(bufferSeconds), bufferSeconds, color))
		out.WriteString("h")
	}

	if verbose && epoch > 0 {
		out.WriteString(fmt.Sprintf(" epoch:%d(%s)", epoch, time.Unix(epoch, 0)))
	}

	return out.String()
}

func dayMarker(epoch int64, values map[string]int64, weekendDays map[string]struct{}) string {
	if values["off"] >= 8*secondsPerHour {
		return "*"
	}
	if values["bank"] >= 8*secondsPerHour {
		return "*"
	}

	weekday := time.Unix(epoch, 0).Format("Mon")
	if _, ok := weekendDays[weekday]; ok {
		return "*"
	}

	return " "
}

func dayLabel(epoch int64) string {
	t := time.Unix(epoch, 0)
	_, week := t.ISOWeek()
	return fmt.Sprintf("%s %s %02d", t.Format("Mon"), t.Format("20060102"), week)
}

func dayKey(epoch int64) string {
	t := time.Unix(epoch, 0)
	return t.Format("2006-01-02")
}

func weekLabel(epoch int64) string {
	_, week := time.Unix(epoch, 0).ISOWeek()
	return fmt.Sprintf("%02d", week)
}

func isoWeekKey(epoch int64) string {
	year, week := time.Unix(epoch, 0).ISOWeek()
	return fmt.Sprintf("%d-%02d", year, week)
}

func reportDefaults(cfg config.Config) config.Config {
	defaults := config.Default()

	if cfg.WeekWorkHours == 0 {
		cfg.WeekWorkHours = defaults.WeekWorkHours
	}
	if cfg.PlusFor == nil {
		cfg.PlusFor = defaults.PlusFor
	}
	if cfg.WeekendDays == nil {
		cfg.WeekendDays = defaults.WeekendDays
	}
	if cfg.MinusFor == nil {
		cfg.MinusFor = defaults.MinusFor
	}
	if cfg.BufferFor == nil {
		cfg.BufferFor = defaults.BufferFor
	}

	return cfg
}

func formatHours(seconds int64) string {
	return fmt.Sprintf("%0.2f", float64(seconds)/float64(secondsPerHour))
}

func colorizeLabel(label string, color bool) string {
	if !color {
		return label
	}
	return colorCyan + label + colorReset
}

func colorizeValue(value string, raw int64, color bool) string {
	if !color {
		return value
	}

	switch {
	case raw < 0:
		return colorRed + value + colorReset
	case raw > 0:
		return colorGreen + value + colorReset
	default:
		return value
	}
}

func cloneValueMap(values map[string]int64) map[string]int64 {
	cloned := make(map[string]int64, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func stringSet(items []string) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		set[trimmed] = struct{}{}
	}
	return set
}

func newDayAccumulator() dayAccumulator {
	return dayAccumulator{
		values: map[string]int64{},
	}
}

func newWeekAccumulator() weekAccumulator {
	return weekAccumulator{
		values: map[string]int64{},
	}
}

func sortedWeekReports(reports []WeekReport) {
	sort.SliceStable(reports, func(i, j int) bool {
		if reports[i].WeekLabel != reports[j].WeekLabel {
			return reports[i].WeekLabel < reports[j].WeekLabel
		}
		if len(reports[i].Days) == 0 || len(reports[j].Days) == 0 {
			return len(reports[i].Days) < len(reports[j].Days)
		}
		return reports[i].Days[0].Epoch < reports[j].Days[0].Epoch
	})
}
