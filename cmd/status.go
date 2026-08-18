package cmd

import (
	"github.com/spf13/cobra"

	"github.com/stephenc/gits/internal/runner"
)

func newStatusCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Display a summary of branch statuses",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(opts)
		},
	}
}

func runStatus(opts *options) error {
	if runner.RunStatus(opts.filters(), opts.parallel) != 0 {
		return errExit
	}
	return nil
}
