package cmd

import (
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/stephenc/gits/internal/git"
	"github.com/stephenc/gits/internal/runner"
)

// registerFilterCompletions teaches the shell completion the values that the
// filter flags could usefully take, gathered by walking the repositories below
// the current directory. The gathering parses .git files directly rather than
// running git, so a completion request stays fast even across many repos.
func registerFilterCompletions(root *cobra.Command) {
	completions := map[string]func() []string{
		"name-contains":   repoNames,
		"name-starts":     repoNames,
		"name-ends":       repoNames,
		"branch":          currentBranches,
		"remote-name":     remoteNames,
		"remote-host":     remoteHosts,
		"remote-contains": remoteURLs,
	}

	for flag, values := range completions {
		_ = root.RegisterFlagCompletionFunc(flag, completeFrom(values))
	}
}

// completeFrom adapts a candidate gatherer to cobra's completion signature.
// Candidates are offered unfiltered: the shell narrows by what the user typed,
// and for contains/ends style flags a full value is still the best prompt.
func completeFrom(values func() []string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return values(), cobra.ShellCompDirectiveNoFileComp
	}
}

// repos returns the repositories below the current directory, unfiltered.
func repos() []string {
	repos, _, err := runner.Discover(nil)
	if err != nil {
		return nil
	}
	return repos
}

// gather collects the deduplicated, sorted union of extract over every
// repository below the current directory.
func gather(extract func(path string) []string) []string {
	seen := map[string]bool{}
	for _, repo := range repos() {
		for _, v := range extract(repo) {
			if v != "" {
				seen[v] = true
			}
		}
	}
	values := make([]string, 0, len(seen))
	for v := range seen {
		values = append(values, v)
	}
	sort.Strings(values)
	return values
}

func repoNames() []string {
	return gather(func(path string) []string {
		return []string{filepath.Base(path)}
	})
}

func currentBranches() []string {
	return gather(func(path string) []string {
		return []string{git.HeadBranch(path)}
	})
}

func remoteNames() []string {
	return gather(func(path string) []string {
		names, _ := git.ConfigRemotes(path)
		return names
	})
}

func remoteURLs() []string {
	return gather(func(path string) []string {
		_, urls := git.ConfigRemotes(path)
		return urls
	})
}

func remoteHosts() []string {
	return gather(func(path string) []string {
		_, urls := git.ConfigRemotes(path)
		hosts := make([]string, 0, len(urls))
		for _, u := range urls {
			hosts = append(hosts, git.RemoteHost(u))
		}
		return hosts
	})
}
