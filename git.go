package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
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
