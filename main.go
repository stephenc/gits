package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func isGitRepo(path string) bool {
	gitDir := filepath.Join(path, ".git")
	info, err := os.Stat(gitDir)
	return err == nil && info.IsDir()
}

func getCurrentBranch(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func getDefaultBranch(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "config", "get", "init.defaultbranch")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	res := strings.TrimSpace(string(out))
	if res == "" {
		return "main", nil
	}

	return res, nil
}

func getLocalBranches(path string) ([]string, error) {
	cmd := exec.Command("git", "-C", path, "branch", "--format", "%(refname:short)")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	names := strings.Split(string(out), "\n")

	res := make([]string, 0, len(names))

	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		res = append(res, n)
	}
	return res, nil
}

func isDirty(path string) (bool, error) {
	cmd := exec.Command("git", "-C", path, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(out) > 0, nil
}

func isClean(path string) (bool, error) {
	cmd := exec.Command("git", "-C", path, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(out) == 0, nil
}

func getStashCount(path string) (int, error) {
	cmd := exec.Command("git", "-C", path, "stash", "list", "--format=%h")
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return 0, err
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return 0, nil // No stashes
	}
	return len(lines), nil
}

func isStash(path string) (bool, error) {
	count, err := getStashCount(path)
	return count > 0, err
}

func getRemoteURLs(path string) ([]string, error) {
	cmd := exec.Command("git", "-C", path, "remote", "-v")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var urls []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if !slices.Contains(urls, fields[1]) {
			urls = append(urls, fields[1])
		}
	}
	return urls, nil
}

func getRemoteNames(path string) ([]string, error) {
	cmd := exec.Command("git", "-C", path, "remote")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		names = append(names, line)
	}
	return names, nil
}

func remoteHost(rawURL string) string {
	if i := strings.Index(rawURL, "://"); i >= 0 {
		host := rawURL[i+3:]
		if j := strings.Index(host, "/"); j >= 0 {
			host = host[:j]
		}
		if j := strings.LastIndex(host, "@"); j >= 0 {
			host = host[j+1:]
		}
		if j := strings.Index(host, ":"); j >= 0 {
			host = host[:j]
		}
		return host
	}
	// scp-like syntax: [user@]host:path
	if j := strings.Index(rawURL, ":"); j >= 0 {
		host := rawURL[:j]
		if k := strings.Index(host, "@"); k >= 0 {
			host = host[k+1:]
		}
		return host
	}
	return "" // local path, no host
}

type RemoteSyncState int

const (
	BehindRemote RemoteSyncState = -1
	SyncRemote   RemoteSyncState = 0
	AheadRemote  RemoteSyncState = 1
)

func getRemoteSyncStatus(path string) (RemoteSyncState, error) {
	cmd := exec.Command("git", "-C", path, "status", "--porcelain", "--branch")
	out, err := cmd.Output()
	if err != nil {
		return SyncRemote, err
	}

	firstLine := strings.SplitN(string(out), "\n", 2)[0]

	if !strings.HasPrefix(firstLine, "##") {
		return SyncRemote, fmt.Errorf("first line `%s` does not start with expected `##`", firstLine)
	}

	if strings.Contains(firstLine, "[behind") {
		return BehindRemote, nil
	}
	if strings.Contains(firstLine, "[ahead") {
		return AheadRemote, nil
	}
	return SyncRemote, nil
}

func runCommand(path string, command []string) (string, int) {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = path
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
		}
	}
	return out.String(), exitCode
}

type filter func(path string) (bool, error)

func processRepo(wg *sync.WaitGroup, mu *sync.Mutex, path string, cwd string, command []string, quiet bool, results *[]string, finalExitCode *int) {
	defer wg.Done()

	// Run the command
	relPath, err := filepath.Rel(cwd, path)
	if err != nil {
		relPath = path
	}
	output, exitCode := runCommand(path, command)
	status := "✅️" // Checkmark
	if exitCode != 0 {
		status = "❌" // Cross mark
		*finalExitCode = 1
	}

	var result string
	if quiet && exitCode == 0 {
		result = fmt.Sprintf("\033[1m%s %s\033[0m", status, relPath)
	} else {
		result = fmt.Sprintf("\033[1m%s %s:\033[0m\n  %s", status, relPath, strings.ReplaceAll(output, "\n", "\n  "))
	}

	mu.Lock()
	*results = append(*results, result)
	mu.Unlock()
}

func statusRepo(wg *sync.WaitGroup, mu *sync.Mutex, path string, cwd string, width int, results *[]string, finalExitCode *int) {
	defer wg.Done()

	relPath, err := filepath.Rel(cwd, path)
	if err != nil {
		relPath = path
	}

	currentBranch, err := getCurrentBranch(path)
	if err != nil {
		currentBranch = "!" + err.Error()
	}

	defaultBranch, err := getDefaultBranch(path)
	if err != nil {
		defaultBranch = "main"
	}

	remoteSync, err := getRemoteSyncStatus(path)
	if err != nil {
		remoteSync = SyncRemote
	}

	clean, err := isClean(path)
	if err != nil {
		clean = false
	}

	stashes, err := getStashCount(path)
	if err != nil {
		stashes = 0
	}

	localBranches, err := getLocalBranches(path)
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
	case BehindRemote:
		status.WriteString("😰")
	case AheadRemote:
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

	result := fmt.Sprintf("\033[1m%"+strconv.Itoa(-width)+"s\033[0m%s", relPath, branches.String())

	mu.Lock()
	*results = append(*results, result)
	mu.Unlock()
}

func main() {
	parallel := flag.Int("parallel", runtime.NumCPU(), "number of parallel tasks")
	branch := flag.String("branch", "", "only match repositories on this branch")
	dirty := flag.Bool("dirty", false, "only match repositories with a dirty worktree")
	clean := flag.Bool("clean", false, "only match repositories with a clean worktree")
	stash := flag.Bool("stash", false, "only match repositories with stashed changes")
	remote := flag.Bool("remote", false, "only match repositories with at least one remote")
	noRemote := flag.Bool("no-remote", false, "only match repositories with no remotes")
	remoteContains := flag.String("remote-contains", "", "only match repositories with a remote URL containing this substring")
	remoteHostname := flag.String("remote-host", "", "only match repositories with a remote on this host")
	remoteName := flag.String("remote-name", "", "only match repositories with a remote of this name")
	quiet := flag.Bool("quiet", false, "suppress output from repositories where the command succeeded")
	help := flag.Bool("help", false, "display help message")
	status := flag.Bool("status", false, "display a summary of branch statuses and exit")
	flag.Parse()

	if *help {
		fmt.Println("Usage: gits [options] command [args...]")
		flag.PrintDefaults()
		os.Exit(0)
	}

	var filters []filter

	if *branch != "" {
		filters = append(filters, func(path string) (bool, error) {
			b, err := getCurrentBranch(path)
			return b == *branch, err
		})
	}

	if *dirty {
		filters = append(filters, isDirty)
	}

	if *clean {
		filters = append(filters, isClean)
	}

	if *stash {
		filters = append(filters, isStash)
	}

	if *remote {
		filters = append(filters, func(path string) (bool, error) {
			urls, err := getRemoteURLs(path)
			return len(urls) > 0, err
		})
	}

	if *noRemote {
		filters = append(filters, func(path string) (bool, error) {
			urls, err := getRemoteURLs(path)
			return len(urls) == 0, err
		})
	}

	if *remoteContains != "" {
		filters = append(filters, func(path string) (bool, error) {
			urls, err := getRemoteURLs(path)
			if err != nil {
				return false, err
			}
			return slices.ContainsFunc(urls, func(u string) bool {
				return strings.Contains(u, *remoteContains)
			}), nil
		})
	}

	if *remoteHostname != "" {
		filters = append(filters, func(path string) (bool, error) {
			urls, err := getRemoteURLs(path)
			if err != nil {
				return false, err
			}
			return slices.ContainsFunc(urls, func(u string) bool {
				return remoteHost(u) == *remoteHostname
			}), nil
		})
	}

	if *remoteName != "" {
		filters = append(filters, func(path string) (bool, error) {
			names, err := getRemoteNames(path)
			if err != nil {
				return false, err
			}
			return slices.Contains(names, *remoteName), nil
		})
	}

	var applyAction func(wg *sync.WaitGroup, mu *sync.Mutex, path string, cwd string, results *[]string, finalExitCode *int)

	var gitRepos []string

	if *status {
		applyAction = func(wg *sync.WaitGroup, mu *sync.Mutex, path string, cwd string, results *[]string, finalExitCode *int) {
			var longestName int = 0
			for _, repo := range gitRepos {
				relPath, err := filepath.Rel(cwd, repo)
				if err != nil {
					relPath = path
				}

				if len(relPath) > longestName {
					longestName = len(relPath)
				}
			}

			statusRepo(wg, mu, path, cwd, longestName, results, finalExitCode)
		}
	} else {
		command := flag.Args()
		if len(command) == 0 {
			fmt.Println("No command provided")
			flag.PrintDefaults()
			os.Exit(1)
		}

		applyAction = func(wg *sync.WaitGroup, mu *sync.Mutex, path string, cwd string, results *[]string, finalExitCode *int) {
			processRepo(wg, mu, path, cwd, command, *quiet, results, finalExitCode)
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting current working directory:", err)
		os.Exit(1)
	}

	// Resolve symlink
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		fmt.Println("Error resolving symlink:", err)
		os.Exit(1)
	}

	err = filepath.Walk(cwd, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && isGitRepo(path) {
			// Check filters
			for _, f := range filters {
				r, err := f(path)
				if err != nil || !r {
					return filepath.SkipDir
				}
			}

			gitRepos = append(gitRepos, path)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		fmt.Println("Error walking the path:", err)
		os.Exit(1)
	}

	sort.Strings(gitRepos)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []string
	finalExitCode := 0

	totalTasks := len(gitRepos)
	remainingTasks := totalTasks

	sem := make(chan struct{}, *parallel)
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

	for _, repo := range gitRepos {
		wg.Add(1)
		sem <- struct{}{}
		go func(repo string) {
			defer func() { <-sem }()
			applyAction(&wg, &mu, repo, cwd, &results, &finalExitCode)
			remainingTasks--
		}(repo)
	}

	wg.Wait()
	close(sem)

	fmt.Print("\r                      \r")

	sort.Strings(results)
	for _, result := range results {
		fmt.Println(result)
	}

	os.Exit(finalExitCode)
}
