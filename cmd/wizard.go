package cmd

import (
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/JustSteveKing/flint/internal/tui"
)

// runWizard is the one way into the walkthrough. Both `flint` and
// `flint flash <file>` end up here; the flags only decide which steps are
// already answered.
func runWizard(cmd *cobra.Command, opts tui.Options) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	model := tui.New(ctx, opts)
	if _, err := tea.NewProgram(model, tea.WithContext(ctx), tea.WithAltScreen()).Run(); err != nil {
		return err
	}
	return model.Err()
}
