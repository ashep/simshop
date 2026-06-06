package main

import (
	"fmt"
	"os"

	"github.com/ashep/simshop/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
