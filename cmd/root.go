// Package cmd wires flint's commands.
package cmd

import (
	"github.com/spf13/cobra"
)

// version is overwritten at build time with -ldflags.
var version = "dev"

// NewRoot builds the command tree.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "flint",
		Short: "Flash keyboard firmware on Linux",
		Long: `flint watches USB for a keyboard dropping into bootloader mode, tells you
what it found, and flashes it with the right tool.

It does not implement DFU. dfu-util and wb32-dfu-updater_cli do that part.
What flint adds is the loop around them, which is the bit QMK Toolbox has on
Windows and macOS and Linux does not.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(
		newDevicesCmd(),
		newBoardsCmd(),
		newWatchCmd(),
		newFlashCmd(),
	)
	return root
}
