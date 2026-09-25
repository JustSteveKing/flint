package cmd

import (
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/JustSteveKing/flint/internal/board"
	"github.com/JustSteveKing/flint/internal/usb"
)

func newWatchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Print USB devices as they come and go, without flashing anything",
		Long: `watch reports every USB device appearing and disappearing, and says which
ones flint recognises.

This is how you find out what a new board's bootloader enumerates as. Run it,
put the keyboard into bootloader mode, and read the ID off the line that
appears. Nothing is written to the device.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			fmt.Println("Watching USB. Put a board into bootloader mode. Ctrl-C to stop.")
			fmt.Println()

			for ev := range usb.Watch(ctx, usb.DefaultInterval) {
				if ev.Err != nil {
					fmt.Printf("%s  error: %v\n", time.Now().Format("15:04:05"), ev.Err)
					continue
				}

				sign := "+"
				if ev.Kind == usb.Removed {
					sign = "-"
				}

				note := describe(ev.Device.ID)
				fmt.Printf("%s  %s %s  %-34s %s\n",
					time.Now().Format("15:04:05"),
					sign,
					ev.Device.ID,
					truncate(ev.Device.Label(), 34),
					note,
				)
			}

			return ctx.Err()
		},
	}
}

func describe(id usb.ID) string {
	if bl, ok := board.BootloaderByID(id); ok {
		candidates := board.CandidatesFor(bl)
		if len(candidates) == 1 {
			return "<- bootloader (" + bl.Name + "), almost certainly " + candidates[0].Name
		}
		return "<- bootloader (" + bl.Name + ")"
	}
	if b, ok := board.ByNormalID(id); ok {
		return "<- " + b.Name + ", running normally"
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
