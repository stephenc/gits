// Package git wraps the git invocations that gits uses to inspect and act on
// repositories.
package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// IsRepo reports whether path is the root of a git repository.
func IsRepo(path string) bool {
	gitDir := filepath.Join(path, ".git")
	info, err := os.Stat(gitDir)
	return err == nil && info.IsDir()
}

// CurrentBranch returns the branch that the repository at path has checked out.
func CurrentBranch(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// DefaultBranch returns the configured init.defaultbranch, or "main" when
// nothing is configured.
func DefaultBranch(path string) (string, error) {
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

// LocalBranches returns the names of the local branches of the repository at
// path.
func LocalBranches(path string) ([]string, error) {
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

// IsDirty reports whether the worktree of the repository at path has changes.
func IsDirty(path string) (bool, error) {
	cmd := exec.Command("git", "-C", path, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(out) > 0, nil
}

// IsClean reports whether the worktree of the repository at path has no changes.
func IsClean(path string) (bool, error) {
	cmd := exec.Command("git", "-C", path, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(out) == 0, nil
}

// StashCount returns the number of stashes in the repository at path.
func StashCount(path string) (int, error) {
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

// RemoteURLs returns the distinct remote URLs of the repository at path.
func RemoteURLs(path string) ([]string, error) {
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

// RemoteNames returns the names of the remotes of the repository at path.
func RemoteNames(path string) ([]string, error) {
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

// RemoteHost extracts the host from a remote URL, or returns "" for a local
// path.
func RemoteHost(rawURL string) string {
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

// RemoteSyncState describes whether a branch is behind, in sync with, or
// ahead of its remote tracking branch.
type RemoteSyncState int

const (
	BehindRemote RemoteSyncState = -1
	SyncRemote   RemoteSyncState = 0
	AheadRemote  RemoteSyncState = 1
)

// RemoteSyncStatus reports whether the current branch of the repository at
// path is behind, in sync with, or ahead of its remote tracking branch.
func RemoteSyncStatus(path string) (RemoteSyncState, error) {
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

// RunCommand runs command in the directory at path and returns its combined
// output and exit code.
func RunCommand(path string, command []string) (string, int) {
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
