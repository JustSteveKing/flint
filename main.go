package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/JustSteveKing/flint/cmd"
)

func main() {
	if err := cmd.NewRoot().ExecuteContext(context.Background()); err != nil {
		if !errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "flint:", err)
		}
		os.Exit(1)
	}
}
