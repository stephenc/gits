// Package runner discovers the repositories below the current directory and
// runs work across them in parallel.
package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stephenc/gits/internal/filter"
	"github.com/stephenc/gits/internal/git"
)

// findGitRepos walks the current working directory and returns the git
// repositories that pass every filter, sorted by path.
func findGitRepos(filters []filter.Filter) (repos []string, cwd string, err error) {
	cwd, err = os.Getwd()
	if err != nil {
		return nil, "", fmt.Errorf("error getting current working directory: %w", err)
	}

	// Resolve symlink
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return nil, "", fmt.Errorf("error resolving symlink: %w", err)
	}

	err = filepath.Walk(cwd, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && git.IsRepo(path) {
			// Check filters
			for _, f := range filters {
				r, err := f.Matches(path)
				if err != nil || !r {
					return filepath.SkipDir
				}
			}

			repos = append(repos, path)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, "", fmt.Errorf("error walking the path: %w", err)
	}

	sort.Strings(repos)
	return repos, cwd, nil
}

func processRepo(path string, cwd string, command []string, quiet bool) (string, int) {
	relPath, err := filepath.Rel(cwd, path)
	if err != nil {
		relPath = path
	}
	output, exitCode := git.RunCommand(path, command)
	status := "✅️" // Checkmark
	if exitCode != 0 {
		status = "❌" // Cross mark
	}

	if quiet && exitCode == 0 {
		return fmt.Sprintf("\033[1m%s %s\033[0m", status, relPath), exitCode
	}
	return fmt.Sprintf("\033[1m%s %s:\033[0m\n  %s", status, relPath, strings.ReplaceAll(output, "\n", "\n  ")), exitCode
}

func statusRepo(path string, cwd string, width int) string {
	relPath, err := filepath.Rel(cwd, path)
	if err != nil {
		relPath = path
	}

	currentBranch, err := git.CurrentBranch(path)
	if err != nil {
		currentBranch = "!" + err.Error()
	}

	defaultBranch, err := git.DefaultBranch(path)
	if err != nil {
		defaultBranch = "main"
	}

	remoteSync, err := git.RemoteSyncStatus(path)
	if err != nil {
		remoteSync = git.SyncRemote
	}

	clean, err := git.IsClean(path)
	if err != nil {
		clean = false
	}

	stashes, err := git.StashCount(path)
	if err != nil {
		stashes = 0
	}

	localBranches, _ := git.LocalBranches(path)
	localBranches = slices.DeleteFunc(localBranches, func(x string) bool { return x == currentBranch })
	sort.Strings(localBranches)

	var branches strings.Builder
	if currentBranch == defaultBranch {
		branches.WriteString(" [\033[1;32m")
	} else {
		branches.WriteString(" [\033[1;31m")
	}
	branches.WriteString(currentBranch)
	branches.WriteString("\033[0m]")

	var status strings.Builder
	if !clean {
		status.WriteString("📝")
	}
	if stashes > 0 {
		status.WriteString(strings.Repeat("🥖", stashes))
	}
	switch remoteSync {
	case git.BehindRemote:
		status.WriteString("😰")
	case git.AheadRemote:
		status.WriteString("🏎💨")
	}

	if status.Len() > 0 {
		branches.WriteString("\u001B[31m(\u001B[0m")
		branches.WriteString(status.String())
		branches.WriteString("\u001B[31m)\u001B[0m")
	}

	for _, name := range localBranches {
		branches.WriteString(" [\033[34m")
		branches.WriteString(name)
		branches.WriteString("\033[0m]")
	}

	return fmt.Sprintf("\033[1m%"+strconv.Itoa(-width)+"s\033[0m%s", relPath, branches.String())
}

// forEachRepo runs action against every repo with the requested parallelism,
// showing a progress spinner, then prints the sorted results. It returns the
// worst exit code any action reported.
func forEachRepo(repos []string, parallel int, action func(path string) (string, int)) int {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []string
	finalExitCode := 0

	totalTasks := len(repos)
	remainingTasks := totalTasks

	sem := make(chan struct{}, parallel)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	go func() {
		dots := "."
		for range ticker.C {
			fmt.Printf("\r⚡️ %d/%d %s   \b\b\b", totalTasks-remainingTasks, totalTasks, dots)
			dots = dots + "."
			if len(dots) > 3 {
				dots = "."
			}
		}
	}()

	for _, repo := range repos {
		wg.Add(1)
		sem <- struct{}{}
		go func(repo string) {
			defer wg.Done()
			defer func() { <-sem }()
			result, exitCode := action(repo)

			mu.Lock()
			results = append(results, result)
			if exitCode != 0 {
				finalExitCode = 1
			}
			remainingTasks--
			mu.Unlock()
		}(repo)
	}

	wg.Wait()
	close(sem)

	fmt.Print("\r                      \r")

	sort.Strings(results)
	for _, result := range results {
		fmt.Println(result)
	}

	return finalExitCode
}

// RunAcross runs command in every matching repository and returns the exit
// code the process should finish with.
func RunAcross(filters []filter.Filter, parallel int, quiet bool, command []string) int {
	repos, cwd, err := findGitRepos(filters)
	if err != nil {
		fmt.Println(err)
		return 1
	}

	return forEachRepo(repos, parallel, func(path string) (string, int) {
		return processRepo(path, cwd, command, quiet)
	})
}

// RunStatus prints a branch status summary for every matching repository and
// returns the exit code the process should finish with.
func RunStatus(filters []filter.Filter, parallel int) int {
	repos, cwd, err := findGitRepos(filters)
	if err != nil {
		fmt.Println(err)
		return 1
	}

	longestName := 0
	for _, repo := range repos {
		relPath, err := filepath.Rel(cwd, repo)
		if err != nil {
			relPath = repo
		}

		if len(relPath) > longestName {
			longestName = len(relPath)
		}
	}

	return forEachRepo(repos, parallel, func(path string) (string, int) {
		return statusRepo(path, cwd, longestName), 0
	})
}
