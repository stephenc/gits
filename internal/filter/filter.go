// Package filter selects which repositories gits should act on.
package filter

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/stephenc/gits/internal/git"
)

// Filter decides whether a repository at path should be included.
type Filter interface {
	Matches(path string) (bool, error)
}

// Branch matches repositories whose current branch is branch.
func Branch(branch string) Filter {
	return branchFilter{branch: branch}
}

type branchFilter struct {
	branch string
}

func (f branchFilter) Matches(path string) (bool, error) {
	b, err := git.CurrentBranch(path)
	return b == f.branch, err
}

// Dirty matches repositories with a dirty worktree.
func Dirty() Filter {
	return dirtyFilter{}
}

type dirtyFilter struct{}

func (dirtyFilter) Matches(path string) (bool, error) {
	return git.IsDirty(path)
}

// Clean matches repositories with a clean worktree.
func Clean() Filter {
	return cleanFilter{}
}

type cleanFilter struct{}

func (cleanFilter) Matches(path string) (bool, error) {
	return git.IsClean(path)
}

// Stash matches repositories with at least one stash.
func Stash() Filter {
	return stashFilter{}
}

type stashFilter struct{}

func (stashFilter) Matches(path string) (bool, error) {
	count, err := git.StashCount(path)
	return count > 0, err
}

// HasRemote matches repositories with at least one remote.
func HasRemote() Filter {
	return hasRemoteFilter{}
}

type hasRemoteFilter struct{}

func (hasRemoteFilter) Matches(path string) (bool, error) {
	urls, err := git.RemoteURLs(path)
	return len(urls) > 0, err
}

// NoRemote matches repositories with no remotes.
func NoRemote() Filter {
	return noRemoteFilter{}
}

type noRemoteFilter struct{}

func (noRemoteFilter) Matches(path string) (bool, error) {
	urls, err := git.RemoteURLs(path)
	return len(urls) == 0, err
}

// RemoteContains matches repositories with a remote URL containing substring.
func RemoteContains(substring string) Filter {
	return remoteContainsFilter{substring: substring}
}

type remoteContainsFilter struct {
	substring string
}

func (f remoteContainsFilter) Matches(path string) (bool, error) {
	urls, err := git.RemoteURLs(path)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(urls, func(u string) bool {
		return strings.Contains(u, f.substring)
	}), nil
}

// RemoteHost matches repositories with a remote on host.
func RemoteHost(host string) Filter {
	return remoteHostFilter{host: host}
}

type remoteHostFilter struct {
	host string
}

func (f remoteHostFilter) Matches(path string) (bool, error) {
	urls, err := git.RemoteURLs(path)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(urls, func(u string) bool {
		return git.RemoteHost(u) == f.host
	}), nil
}

// RemoteName matches repositories with a remote named name.
func RemoteName(name string) Filter {
	return remoteNameFilter{name: name}
}

type remoteNameFilter struct {
	name string
}

func (f remoteNameFilter) Matches(path string) (bool, error) {
	names, err := git.RemoteNames(path)
	if err != nil {
		return false, err
	}
	return slices.Contains(names, f.name), nil
}

// NameContains matches repositories whose directory name contains substring.
func NameContains(substring string) Filter {
	return nameContainsFilter{substring: substring}
}

type nameContainsFilter struct {
	substring string
}

func (f nameContainsFilter) Matches(path string) (bool, error) {
	return strings.Contains(filepath.Base(path), f.substring), nil
}

// NameStarts matches repositories whose directory name starts with prefix.
func NameStarts(prefix string) Filter {
	return nameStartsFilter{prefix: prefix}
}

type nameStartsFilter struct {
	prefix string
}

func (f nameStartsFilter) Matches(path string) (bool, error) {
	return strings.HasPrefix(filepath.Base(path), f.prefix), nil
}

// NameEnds matches repositories whose directory name ends with suffix.
func NameEnds(suffix string) Filter {
	return nameEndsFilter{suffix: suffix}
}

type nameEndsFilter struct {
	suffix string
}

func (f nameEndsFilter) Matches(path string) (bool, error) {
	return strings.HasSuffix(filepath.Base(path), f.suffix), nil
}
