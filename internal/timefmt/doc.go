// Package timefmt parses durations, timestamps, and date ranges for timesamurai.
//
// Durations keep the worktime.rb convention that a bare integer is seconds.
// Times accept clock values, relative offsets, calendar keywords, and legacy
// unix epochs. Ranges cover named windows (today, week, …), YYYY-MM months,
// and inclusive date..date spans.
package timefmt
