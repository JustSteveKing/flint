package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/JustSteveKing/flint/internal/board"
	"github.com/JustSteveKing/flint/internal/flash"
	"github.com/JustSteveKing/flint/internal/tui"
)

func newFlashCmd() *cobra.Command {
	var (
		dryRun    bool
		boardName string
	)

	cmd := &cobra.Command{
		Use:   "flash <firmware>",
		Short: "Start the walkthrough with the firmware already chosen",
		Long: `flash starts the same walkthrough as running flint bare, with the file
step already answered.

Adding --board answers the board step too, which drops you straight onto the
instructions for putting that keyboard into bootloader mode.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			firmware := args[0]

			// Check the file before opening a full-screen UI, so a typo fails
			// as a plain error rather than inside the TUI.
			if err := flash.CheckFirmware(firmware); err != nil {
				return err
			}

			opts := tui.Options{Firmware: firmware, DryRun: dryRun}
			if boardName != "" {
				b, err := findBoard(boardName)
				if err != nil {
					return err
				}
				opts.Board = &b
			}

			return runWizard(cmd, opts)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "do everything except write to the device")
	cmd.Flags().StringVarP(&boardName, "board", "b", "", "restrict to one board by name, e.g. \"Q1 HE\"")
	return cmd
}

// findBoard matches on any unambiguous substring, so "q1" is enough.
func findBoard(name string) (board.Board, error) {
	needle := strings.ToLower(name)

	var matches []board.Board
	for _, b := range board.Known {
		if strings.Contains(strings.ToLower(b.Name), needle) {
			matches = append(matches, b)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return board.Board{}, fmt.Errorf("no board matching %q; try `flint boards`", name)
	default:
		names := make([]string, len(matches))
		for i, b := range matches {
			names[i] = b.Name
		}
		return board.Board{}, fmt.Errorf("%q matches several boards: %s", name, strings.Join(names, ", "))
	}
}
