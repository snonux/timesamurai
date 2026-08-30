package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// This file owns the $EDITOR process/temp-file plumbing `work edit` (edit.go)
// needs: writing the rendered block to a scratch file, spawning $EDITOR
// against it, and reading back whatever the editor left behind. It knows
// nothing about the edit-block format or how the result gets applied -- it
// just moves a string out to a file, through an external editor, and back.

// launchEditor writes content to a scratch file, runs $EDITOR on it
// (inheriting cmd's stdin/stdout/stderr so an interactive editor works from
// the terminal), and returns the file's contents afterward. An unset
// $EDITOR fails clearly rather than falling back to a guessed default: a
// silently-chosen editor the user didn't ask for is worse than an explicit
// error naming the fix ("set $EDITOR").
func launchEditor(cmd *cobra.Command, content string) (string, error) {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		return "", errors.New("$EDITOR is not set; work edit needs an editor to open the entries in")
	}

	path, cleanup, err := writeEditScratchFile(content)
	if err != nil {
		return "", err
	}
	defer cleanup()

	if err := runEditor(cmd, editor, path); err != nil {
		return "", err
	}

	edited, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read edited file: %w", err)
	}
	return string(edited), nil
}

// writeEditScratchFile writes content to a fresh temp file and returns its
// path plus a cleanup func that removes it, so launchEditor can defer
// cleanup regardless of how runEditor or the later read turns out.
func writeEditScratchFile(content string) (string, func(), error) {
	f, err := os.CreateTemp("", "timesamurai-edit-*.txt")
	if err != nil {
		return "", nil, fmt.Errorf("create scratch file: %w", err)
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, fmt.Errorf("write scratch file: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close scratch file: %w", err)
	}
	return path, cleanup, nil
}

// runEditor runs $EDITOR (which may itself carry arguments, e.g. "code
// --wait") against path, wiring cmd's stdio through so an interactive
// terminal editor behaves normally and a test's fake editor script can read
// cmd's injected stdin if it wants to.
func runEditor(cmd *cobra.Command, editor, path string) error {
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return errors.New("$EDITOR is not set; work edit needs an editor to open the entries in")
	}
	args := append(append([]string{}, parts[1:]...), path)
	editorCmd := exec.CommandContext(cmdContext(cmd), parts[0], args...) //nolint:gosec // $EDITOR is operator-controlled, same trust level as running any local tool
	editorCmd.Stdin = cmd.InOrStdin()
	editorCmd.Stdout = cmd.OutOrStdout()
	editorCmd.Stderr = cmd.ErrOrStderr()
	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("run $EDITOR (%s): %w", editor, err)
	}
	return nil
}
