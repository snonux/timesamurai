package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/ansi"
	"github.com/snonux/timesamurai/internal/pathutil"
)

// wisdomsFileName is the bullet list of work wisdoms a random line is drawn
// from, kept in the worktime repo (storage.db_dir, ~/git/worktime by
// default) next to the data it is a reminder about. It replaces the fish
// shell's worktime::wisdom_reminder, so every entry point shows it, not
// only the wrappers that remembered to call it.
const wisdomsFileName = "work-wisdoms.md"

// wisdomPrefix marks a wisdom line: a Markdown bullet. Anything else in the
// file (headings, prose, blank lines) is ignored.
const wisdomPrefix = "* "

// printWisdomAfterAction is the `work` tree's PersistentPostRun hook, so it
// runs after every subcommand that succeeded (cobra skips post-run hooks on
// error). It prints one random wisdom to stderr, and only when stderr is a
// terminal: the line is for a human, and must never end up in piped output
// such as `work report | tee report.txt` or `work export`. A missing
// wisdoms file is the normal "feature not used" case and prints nothing.
func printWisdomAfterAction(cmd *cobra.Command) {
	_, stderr := outputStreams()
	if !ansi.IsTerminal(stderr) {
		return
	}
	cfg := loadAmbientConfig(configFlag(cmd))
	dbDir := cfg.Storage.DBDir
	if db, _ := cmd.Flags().GetString("db"); db != "" {
		if expanded, err := pathutil.ExpandHome(db); err == nil {
			dbDir = expanded
		}
	}
	_, errTheme, err := resolveThemes(cmd, cfg)
	if err != nil {
		errTheme = ansi.Theme{}
	}
	printWisdom(cmd.ErrOrStderr(), errTheme, filepath.Join(dbDir, wisdomsFileName), rand.IntN)
}

// configFlag returns --config as cobra parsed it ("" when the flag does not
// exist on this tree, as in tests that build `work` standalone).
func configFlag(cmd *cobra.Command) string {
	path, _ := cmd.Flags().GetString("config")
	return path
}

// printWisdom writes one wisdom picked from path to w. pick(n) returns an
// index in [0, n); it is injected so tests are deterministic. A file that
// does not exist or holds no wisdom lines prints nothing; any other read
// error prints a warning, since the user evidently set the file up.
func printWisdom(w io.Writer, theme ansi.Theme, path string, pick func(int) int) {
	wisdoms, err := readWisdoms(path)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		_, _ = fmt.Fprintln(w, theme.Wrap(ansi.RoleWarn, fmt.Sprintf("warning: wisdoms %s: %v", path, err)))
		return
	}
	if len(wisdoms) == 0 {
		return
	}
	_, _ = fmt.Fprintln(w, theme.Wrap(ansi.RoleDim, wisdoms[pick(len(wisdoms))]))
}

// readWisdoms returns the text of every "* " bullet line in path, with the
// bullet marker and surrounding whitespace stripped.
func readWisdoms(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var wisdoms []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if text, ok := strings.CutPrefix(sc.Text(), wisdomPrefix); ok {
			if text = strings.TrimSpace(text); text != "" {
				wisdoms = append(wisdoms, text)
			}
		}
	}
	return wisdoms, sc.Err()
}
