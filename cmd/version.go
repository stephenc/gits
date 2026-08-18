package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/stephenc/gits/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Display the version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			printVersion()
		},
	}
}

func printVersion() {
	fmt.Println("gits " + version.String())
}
