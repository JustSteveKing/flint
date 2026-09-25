package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/JustSteveKing/flint/internal/board"
	"github.com/JustSteveKing/flint/internal/usb"
)

func newDevicesCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "devices",
		Short: "List connected USB devices flint recognises",
		RunE: func(cmd *cobra.Command, args []string) error {
			devices, err := usb.List()
			if err != nil {
				return err
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tWHAT\tDEVICE")

			shown := 0
			for _, d := range devices {
				what := ""
				switch {
				case isBootloader(d.ID):
					bl, _ := board.BootloaderByID(d.ID)
					what = "bootloader: " + bl.Name
				case isKnownBoard(d.ID):
					b, _ := board.ByNormalID(d.ID)
					what = "keyboard: " + b.Name
				case !all:
					continue
				default:
					what = "-"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n", d.ID, what, d.Label())
				shown++
			}
			w.Flush()

			if shown == 0 {
				fmt.Println("Nothing recognised. Pass --all to see every USB device.")
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&all, "all", "a", false, "show every USB device, not just recognised ones")
	return cmd
}

func isBootloader(id usb.ID) bool {
	_, ok := board.BootloaderByID(id)
	return ok
}

func isKnownBoard(id usb.ID) bool {
	_, ok := board.ByNormalID(id)
	return ok
}
