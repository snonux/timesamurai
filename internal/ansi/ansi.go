// Package ansi renders terminal colors for timesamurai's CLI output and
// resolves whether colors should be enabled at all (tasks n9 and m83).
//
// The design keeps color decisions out of the domain packages: worktime's
// formatters stay byte-identical (the golden-diff regression guards depend
// on that), and the CLI layer post-processes their plain output through a
// Theme before writing. A disabled Theme is the identity function, so "all
// colors off" means literally every call site collapses to the pre-n9
// output.
//
// Enabling logic follows the de-facto standard precedence:
//
//	NO_COLOR set (any non-empty value)  -> off, no matter what
//	report.color=false / --no-color     -> off, no matter what
//	CLICOLOR_FORCE set and not "0"      -> on, even when not a terminal
//	stdout is not a terminal            -> off
//	otherwise                           -> on
//
// so piping into a file or pager never leaks escape codes, an explicit
// disable (config or --no-color) always wins, and a test (or a user who
// wants colors through a pipe) can force them with CLICOLOR_FORCE.
package ansi

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
)

// Style roles are the semantic names output code refers to. Roles — not raw
// colors — are what config.toml's [colors] palette overrides, so a user
// remaps "what is a warning" once instead of hunting every fmt call.
const (
	RoleHeader    = "header"    // table/report headers: bold
	RoleSeparator = "separator" // rule lines: faint
	RoleSuccess   = "success"   // confirmations: green
	RoleWarn      = "warn"      // warnings, would-delete lines: yellow
	RoleError     = "error"     // errors: red
	RoleAddress   = "address"   // <host>:<id> entry addresses: cyan
	RoleTime      = "time"      // timestamps: blue
	RoleDim       = "dim"       // quiet hints: faint
	RolePositive  = "positive"  // positive balances: green
	RoleNegative  = "negative"  // negative balances: red
	// Task m83 extends coloring from "the highlights" to all text output,
	// which needs roles for the bulk of a report/table rather than only its
	// exceptional parts.
	RoleKey   = "key"   // field names, report categories, actions, help commands/flags: magenta
	RoleValue = "value" // durations and other figures: bold
	RoleTag   = "tag"   // entry tags: yellow
)

// Roles lists every style role in stable order (for docs and error messages).
func Roles() []string {
	return []string{
		RoleHeader, RoleSeparator, RoleSuccess, RoleWarn, RoleError,
		RoleAddress, RoleTime, RoleDim, RolePositive, RoleNegative,
		RoleKey, RoleValue, RoleTag,
	}
}

// DefaultPalette maps each role to its default SGR style name(s). Values are
// space-joined style names parsed by styleCode ("bold cyan").
func DefaultPalette() map[string]string {
	return map[string]string{
		RoleHeader:    "bold",
		RoleSeparator: "faint",
		RoleSuccess:   "green",
		RoleWarn:      "yellow",
		RoleError:     "red",
		RoleAddress:   "cyan",
		RoleTime:      "blue",
		RoleDim:       "faint",
		RolePositive:  "green",
		RoleNegative:  "red",
		RoleKey:       "magenta",
		RoleValue:     "bold",
		RoleTag:       "yellow",
	}
}

// styleCodes maps one style name to its SGR parameter. 30-37/90-97 are the
// 8+8 standard foreground colors; 1/2/3/4 are attribute toggles. Combined
// attribute + color params in one escape (e.g. "1;32") reset together at
// the matching "\x1b[0m", which is fine for our line-scoped wrapping.
var styleCodes = map[string]int{
	"black": 30, "red": 31, "green": 32, "yellow": 33,
	"blue": 34, "magenta": 35, "cyan": 36, "white": 37, "default": 39,
	"brightblack": 90, "brightred": 91, "brightgreen": 92, "brightyellow": 93,
	"brightblue": 94, "brightmagenta": 95, "brightcyan": 96, "brightwhite": 97,
	"bold": 1, "faint": 2, "italic": 3, "underline": 4,
}

// StyleNames lists the valid palette style names in stable order.
func StyleNames() []string {
	return slices.Sorted(maps.Keys(styleCodes))
}

// Theme renders styled text. The zero Theme is valid: disabled, and every
// Wrap returns its input unchanged.
type Theme struct {
	enabled bool
	codes   map[string]string // role -> raw SGR parameter list, e.g. "1;32"
}

// NewTheme builds a Theme. enabled is the master switch (report.color /
// NO_COLOR / TTY resolution — see ShouldEnable). palette overrides the
// default role styles; keys must be valid role names and values valid style
// names ("bold", "red", "bold red", ...), so a typo'd config fails loudly
// at load time instead of silently losing a role.
func NewTheme(enabled bool, palette map[string]string) (Theme, error) {
	theme := Theme{enabled: enabled, codes: map[string]string{}}
	defaults := DefaultPalette()
	for role, style := range palette {
		if !slices.Contains(Roles(), role) {
			return Theme{}, fmt.Errorf("colors.%s: unknown role (want one of %s)",
				role, strings.Join(Roles(), ", "))
		}
		if _, err := styleCode(style); err != nil {
			return Theme{}, fmt.Errorf("colors.%s: %w", role, err)
		}
		defaults[role] = style
	}
	for role, style := range defaults {
		code, err := styleCode(style)
		if err != nil {
			return Theme{}, fmt.Errorf("colors.%s: %w", role, err)
		}
		theme.codes[role] = code
	}
	return theme, nil
}

// styleCode resolves one style name ("bold red") to its SGR parameters.
func styleCode(style string) (string, error) {
	trimmed := strings.TrimSpace(strings.ToLower(style))
	if trimmed == "" {
		return "", nil
	}
	var params []string
	for _, name := range strings.Fields(trimmed) {
		code, ok := styleCodes[name]
		if !ok {
			return "", fmt.Errorf("unknown style %q (want one of %s)",
				name, strings.Join(StyleNames(), ", "))
		}
		params = append(params, strconv.Itoa(code))
	}
	return strings.Join(params, ";"), nil
}

// Enabled reports whether this theme emits escape sequences at all.
func (t Theme) Enabled() bool { return t.enabled }

// Wrap wraps text in role's escape sequence, or returns text unchanged when
// the theme is disabled or role is unknown (an unknown role can only come
// from a programming mistake; rendering it plainly beats crashing a report).
func (t Theme) Wrap(role, text string) string {
	if !t.Enabled() || text == "" {
		return text
	}
	code, ok := t.codes[role]
	if !ok || code == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

// ShouldEnable resolves the standard color enabling precedence for stdout.
// cfgEnabled is the config/env master switch (report.color);
// getenv is injected so tests can simulate the environment; file may be nil
// in tests, which counts as "not a terminal".
func ShouldEnable(cfgEnabled bool, file *os.File, getenv func(string) string) bool {
	if value := getenv("NO_COLOR"); value != "" {
		return false
	}
	if !cfgEnabled {
		return false
	}
	if value := getenv("CLICOLOR_FORCE"); value != "" && value != "0" {
		return true
	}
	return isTerminal(file)
}

// isTerminal reports whether file refers to a character device (a terminal).
func isTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
