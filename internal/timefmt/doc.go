// Package timefmt parses durations, timestamps, and date ranges for timesamurai.
//
// Durations keep the worktime.rb convention that a bare integer is seconds.
// Times accept clock values, relative offsets, calendar keywords (today,
// yesterday, tomorrow, now), human day phrases (next/last/this <weekday>,
// bare weekday names, and "<n> days/weeks/months/years ago"), legacy unix
// epochs, and ISO-8601. A bare integer in --at / ParseTime is treated as a
// unix epoch (same as --epoch), not as an HH or HHMM clock value. Ranges
// cover named windows (today, week, …), YYYY-MM months, and inclusive
// date..date spans.
package timefmt
