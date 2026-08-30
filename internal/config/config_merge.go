package config

import (
	"slices"
	"strings"

	"github.com/snonux/timesamurai/internal/worktime"
)

func (c *Config) mergeWith(other *overlay) {
	if other == nil {
		return
	}
	mergeStorage(&c.Storage, other.Storage)
	mergeAccounting(&c.Accounting, other.Accounting)
	mergeGeneral(&c.General, other.General)
	mergeReport(&c.Report, other.Report)
}

func mergeStorage(dst *StorageConfig, src storageOverlay) {
	if src.StoreDir != nil {
		dst.StoreDir = strings.TrimSpace(*src.StoreDir)
	}
	if src.DBDir != nil {
		dst.DBDir = strings.TrimSpace(*src.DBDir)
	}
}

func mergeAccounting(dst *worktime.AccountingConfig, src accountingOverlay) {
	if src.WeekWorkHours != nil {
		dst.WeekWorkHours = *src.WeekWorkHours
	}
	if src.PlusFor != nil {
		dst.PlusFor = slices.Clone(*src.PlusFor)
	}
	if src.MinusFor != nil {
		dst.MinusFor = slices.Clone(*src.MinusFor)
	}
	if src.BufferFor != nil {
		dst.BufferFor = slices.Clone(*src.BufferFor)
	}
	if src.WeekendDays != nil {
		dst.WeekendDays = slices.Clone(*src.WeekendDays)
	}
}

func mergeGeneral(dst *GeneralConfig, src generalOverlay) {
	if src.Hostname != nil {
		dst.Hostname = strings.TrimSpace(*src.Hostname)
	}
	if src.AutoWorktimeLogin != nil {
		dst.AutoWorktimeLogin = *src.AutoWorktimeLogin
	}
}

func mergeReport(dst *ReportConfig, src reportOverlay) {
	if src.Color != nil {
		dst.Color = *src.Color
	}
	if src.Verbose != nil {
		dst.Verbose = *src.Verbose
	}
}
