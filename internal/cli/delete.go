package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/worktime"
)

// newDeleteCmd builds `work delete <host:id>...`. --dry-run previews without
// calling worktime.Delete; deleting more than one address without --dry-run
// requires interactive confirmation (read from cmd.InOrStdin(), which is
// os.Stdin in normal operation and an in-memory buffer in tests) since a
// batch delete from a mistyped range could otherwise wipe several entries
// with no way back except `work undo` run repeatedly. A single-address
// delete needs no confirmation: there is nothing to conflate it with.
func newDeleteCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "delete <host:id>...",
		Short: "Delete one or more entries",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd, args, dryRun)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be deleted without deleting anything")
	return cmd
}

func runDelete(cmd *cobra.Command, addrs []string, dryRun bool) error {
	rt, err := newRuntime(cmd)
	if err != nil {
		return err
	}

	if dryRun {
		return previewDelete(cmd, rt, addrs)
	}
	if len(addrs) > 1 {
		ok, err := confirmDeleteMultiple(cmd, addrs)
		if err != nil {
			return err
		}
		if !ok {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "delete cancelled")
			return nil
		}
	}
	return performDelete(cmd, rt, addrs)
}

// previewDelete resolves each address against the current entries (without
// mutating anything) and prints what a real delete would remove.
func previewDelete(cmd *cobra.Command, rt *runtime, addrs []string) error {
	rows, err := worktime.Query(worktime.CollectEntries(rt.store), worktime.Filter{})
	if err != nil {
		return err
	}
	for _, addr := range addrs {
		entry, canonical, err := lookupEntry(rows, addr, rt.host)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "would delete %s: %s\n", canonical, formatEntry(entry))
	}
	return nil
}

// performDelete actually deletes each address in order, reporting every
// deleted entry via printEntryResult (the same "<verb> <host>:<id>"/verbose
// shape every other mutating verb uses) so a caller sees exactly what left
// the store. It stops at the first error, leaving any addresses processed
// before it deleted -- each worktime.Delete call is already its own durable,
// undo-logged mutation, so a partial batch is not a partial write.
func performDelete(cmd *cobra.Command, rt *runtime, addrs []string) error {
	for _, addr := range addrs {
		entry, err := worktime.Delete(cmdContext(cmd), rt.store, addr, rt.host)
		if err != nil {
			return fmt.Errorf("delete %s: %w", addr, err)
		}
		printEntryResult(cmd, "delete", entry, rt.verbose)
	}
	return nil
}

// lookupEntry resolves addr to its canonical "<host>:<id>" form and finds
// the matching row in rows (as produced by worktime.Query), so dry-run
// previews and confirmation prompts can show real entry content without a
// second, mutating pass through worktime.Delete.
func lookupEntry(rows []worktime.Row, addr, currentHost string) (worktime.Entry, string, error) {
	host, id, err := worktime.ParseAddress(addr, currentHost)
	if err != nil {
		return worktime.Entry{}, "", err
	}
	canonical := fmt.Sprintf("%s:%d", host, id)
	for _, row := range rows {
		if row.Address == canonical {
			return row.Entry, canonical, nil
		}
	}
	return worktime.Entry{}, "", fmt.Errorf("%w: %s", worktime.ErrEntryNotFound, canonical)
}

// confirmDeleteMultiple prompts on cmd's stdout and reads a yes/no answer
// from cmd.InOrStdin(). Only "y" or "yes" (case-insensitive) confirm;
// anything else, including EOF (an unattended run with nothing on stdin),
// is treated as "no" rather than erroring, so a script that forgets to
// answer safely does nothing instead of accidentally deleting.
func confirmDeleteMultiple(cmd *cobra.Command, addrs []string) (bool, error) {
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "delete %d entries (%s)? [y/N] ", len(addrs), strings.Join(addrs, ", "))
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
