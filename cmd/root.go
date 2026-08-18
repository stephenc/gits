// Package cmd holds the cobra commands that make up the gits CLI.
package cmd

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/stephenc/gits/internal/filter"
	"github.com/stephenc/gits/internal/runner"
)

// errExit signals that Execute should exit with a non-zero code without cobra
// printing an error message: the per-repo output already reports the failures.
var errExit = errors.New("command failed in at least one repository")

type options struct {
	parallel       int
	branch         string
	dirty          bool
	clean          bool
	stash          bool
	remote         bool
	noRemote       bool
	remoteContains string
	remoteHost     string
	remoteName     string
	nameContains   string
	nameStarts     string
	nameEnds       string
	quiet          bool
	status         bool
	version        bool
}

// filters assembles the filters selected by the command line options.
func (opts *options) filters() []filter.Filter {
	var filters []filter.Filter

	if opts.branch != "" {
		filters = append(filters, filter.Branch(opts.branch))
	}
	if opts.dirty {
		filters = append(filters, filter.Dirty())
	}
	if opts.clean {
		filters = append(filters, filter.Clean())
	}
	if opts.stash {
		filters = append(filters, filter.Stash())
	}
	if opts.remote {
		filters = append(filters, filter.HasRemote())
	}
	if opts.noRemote {
		filters = append(filters, filter.NoRemote())
	}
	if opts.remoteContains != "" {
		filters = append(filters, filter.RemoteContains(opts.remoteContains))
	}
	if opts.remoteHost != "" {
		filters = append(filters, filter.RemoteHost(opts.remoteHost))
	}
	if opts.remoteName != "" {
		filters = append(filters, filter.RemoteName(opts.remoteName))
	}
	if opts.nameContains != "" {
		filters = append(filters, filter.NameContains(opts.nameContains))
	}
	if opts.nameStarts != "" {
		filters = append(filters, filter.NameStarts(opts.nameStarts))
	}
	if opts.nameEnds != "" {
		filters = append(filters, filter.NameEnds(opts.nameEnds))
	}

	return filters
}

func newRootCmd() *cobra.Command {
	opts := &options{}

	root := &cobra.Command{
		Use:   "gits [flags] command [args...]",
		Short: "Run a command in every git repository below the current directory",
		Long: `gits walks the current directory for git repositories, optionally narrows
them down with filter flags, and runs the given command in each of them in
parallel.

Any command that is not a gits subcommand is run as-is in each repository, so
"gits gh pr list" just works. To run a command that shares a name with a
subcommand (completion, help, status, version), put it after "--":
"gits -- version".`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.version {
				printVersion()
				return nil
			}
			if opts.status {
				return runStatus(opts)
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return errors.New("no command provided")
			}
			if runner.RunAcross(opts.filters(), opts.parallel, opts.quiet, args) != 0 {
				return errExit
			}
			return nil
		},
	}

	pf := root.PersistentFlags()
	pf.IntVar(&opts.parallel, "parallel", runtime.NumCPU(), "number of parallel tasks")
	pf.StringVar(&opts.branch, "branch", "", "only match repositories on this branch")
	pf.BoolVar(&opts.dirty, "dirty", false, "only match repositories with a dirty worktree")
	pf.BoolVar(&opts.clean, "clean", false, "only match repositories with a clean worktree")
	pf.BoolVar(&opts.stash, "stash", false, "only match repositories with stashed changes")
	pf.BoolVar(&opts.remote, "remote", false, "only match repositories with at least one remote")
	pf.BoolVar(&opts.noRemote, "no-remote", false, "only match repositories with no remotes")
	pf.StringVar(&opts.remoteContains, "remote-contains", "", "only match repositories with a remote URL containing this substring")
	pf.StringVar(&opts.remoteHost, "remote-host", "", "only match repositories with a remote on this host")
	pf.StringVar(&opts.remoteName, "remote-name", "", "only match repositories with a remote of this name")
	pf.StringVar(&opts.nameContains, "name-contains", "", "only match repositories whose directory name contains this substring")
	pf.StringVar(&opts.nameStarts, "name-starts", "", "only match repositories whose directory name starts with this prefix")
	pf.StringVar(&opts.nameEnds, "name-ends", "", "only match repositories whose directory name ends with this suffix")

	root.Flags().BoolVar(&opts.quiet, "quiet", false, "suppress output from repositories where the command succeeded")
	root.Flags().BoolVar(&opts.status, "status", false, "display a summary of branch statuses and exit")
	root.Flags().BoolVar(&opts.version, "version", false, "display the version and exit")

	// The first non-flag argument starts the command to run in each
	// repository; everything after it belongs to that command, even if it
	// looks like a flag.
	root.Flags().SetInterspersed(false)

	root.AddCommand(newStatusCmd(opts), newVersionCmd())

	return root
}

// Execute runs the gits CLI and exits the process with the appropriate code.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		if !errors.Is(err, errExit) {
			fmt.Fprintln(os.Stderr, "Error:", err)
		}
		os.Exit(1)
	}
}
