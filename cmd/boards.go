package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/JustSteveKing/flint/internal/board"
)

func newBoardsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "boards",
		Short: "Show the boards and bootloaders flint knows about",
		RunE: func(cmd *cobra.Command, args []string) error {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

			fmt.Fprintln(w, "BOARD\tRUNNING\tPOSSIBLE BOOTLOADERS")
			for _, b := range board.Known {
				names := make([]string, len(b.Bootloaders))
				for i, bl := range b.Bootloaders {
					names[i] = bl.Name
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n", b.Name, b.Normal, strings.Join(names, ", "))
			}
			fmt.Fprintln(w)

			fmt.Fprintln(w, "BOOTLOADER\tID\tFLASHER\tVERIFIED")
			for _, bl := range board.Bootloaders {
				verified := "no"
				if bl.Verified {
					verified = "yes"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", bl.Name, bl.ID, bl.Flasher, verified)
			}
			return w.Flush()
		},
	}
}
