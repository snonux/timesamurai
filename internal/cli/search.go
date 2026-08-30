package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newSearchCmd builds `work search <text> [filters]`: the same filter
// surface and row rendering as `work list` (see list.go), but text is
// required and drives the Descr substring match instead of a --descr flag --
// query.go's matchesDescr already folds case, so "search" behaves like a
// human would expect ("find" rather than "grep").
func newSearchCmd() *cobra.Command {
	var f filterFlagValues
	cmd := &cobra.Command{
		Use:   "search <text>",
		Short: "Search entry descriptions (case-insensitive substring), addressed for modify/delete",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(cmd, args[0], f)
		},
	}
	addFilterFlags(cmd, &f, false)
	return cmd
}

// runSearch validates the search text, folds it into f.descr (the field
// buildFilter reads regardless of whether it came from --descr or here),
// and otherwise follows exactly the same query+render path as `work list`.
func runSearch(cmd *cobra.Command, text string, f filterFlagValues) error {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return fmt.Errorf("search text must not be empty")
	}
	f.descr = trimmed

	rows, err := queryRows(cmd, "", f)
	if err != nil {
		return err
	}
	return printRows(cmd, rows, f.format)
}
