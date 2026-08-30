// Package pathutil holds small, dependency-free filesystem-path helpers
// shared across timesamurai packages. It exists so generic path logic (like
// tilde expansion) has one home instead of being copied into whichever
// package happens to need it first -- internal/config and internal/cli both
// need to turn a "~"-prefixed path into an absolute one, and neither is the
// right owner of that logic: it's not a configuration concern, and cli
// already depends on config, so putting it here (rather than exporting it
// from config) keeps config's public API focused on configuration and lets
// both callers depend on a package that carries no config-specific baggage.
package pathutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome expands a leading "~" in path to the current user's home
// directory, the same shorthand config.toml path fields and the --db/--store
// CLI flags both accept. An empty path is returned unchanged, and paths
// without a leading "~" (or "~/") pass through unchanged too.
func ExpandHome(path string) (string, error) {
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
