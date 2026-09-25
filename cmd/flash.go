package cmd

import (
	"fmt"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
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
		Short: "Wait for a board in bootloader mode, then flash it",
		Args:  cobra.ExactArgs(1),
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

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			model := tui.New(ctx, opts)
			if _, err := tea.NewProgram(model, tea.WithContext(ctx)).Run(); err != nil {
				return err
			}
			return model.Err()
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
