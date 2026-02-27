package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags "-X main.version=<tag>".
// When built without ldflags (e.g. go run), it defaults to "dev".
var version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the bullarc version",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("bullarc %s\n", version)
	},
}
