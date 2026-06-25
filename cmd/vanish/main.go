package main

import (
	"fmt"
	"os"

	"github.com/itsmeares/vanish/internal/app"
)

func main() {
	// In Go, functions often return an error as their last value. The caller
	// decides how to handle it; for a CLI, printing to stderr and exiting with a
	// non-zero status is a simple, conventional choice.
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "vanish: %v\n", err)
		os.Exit(1)
	}
}
