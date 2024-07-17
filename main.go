package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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

func processRepo(wg *sync.WaitGroup, mu *sync.Mutex, path string, cwd string, command []string, results *[]string, finalExitCode *int) {
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

	result := fmt.Sprintf("\033[1m%s %s:\033[0m\n  %s", status, relPath, strings.ReplaceAll(output, "\n", "\n  "))

	mu.Lock()
	*results = append(*results, result)
	mu.Unlock()
}

func main() {
	parallel := flag.Int("parallel", runtime.NumCPU(), "number of parallel tasks")
	branch := flag.String("branch", "", "only match repositories on this branch")
	dirty := flag.Bool("dirty", false, "only match repositories with a dirty worktree")
	clean := flag.Bool("clean", false, "only match repositories with a clean worktree")
	help := flag.Bool("help", false, "display help message")
	flag.Parse()

	if *help {
		fmt.Println("Usage: gits [options] command [args...]")
		flag.PrintDefaults()
		os.Exit(0)
	}

	var filters []filter

	if *branch != "" {
		filters = append(filters, func(path string) (bool,error) {
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

	command := flag.Args()
	if len(command) == 0 {
		fmt.Println("No command provided")
		flag.PrintDefaults()
		os.Exit(1)
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

	var gitRepos []string
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
			dots = dots+"."
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
			processRepo(&wg, &mu, repo, cwd, command, &results, &finalExitCode)
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
