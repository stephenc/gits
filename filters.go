package main

import (
	"path/filepath"
	"slices"
	"strings"
)

// Filter decides whether a repository at path should be included.
type Filter interface {
	Matches(path string) (bool, error)
}

// branchFilter matches repositories whose current branch is branch.
type branchFilter struct {
	branch string
}

func (f branchFilter) Matches(path string) (bool, error) {
	b, err := getCurrentBranch(path)
	return b == f.branch, err
}

// dirtyFilter matches repositories with a dirty worktree.
type dirtyFilter struct{}

func (dirtyFilter) Matches(path string) (bool, error) {
	return isDirty(path)
}

// cleanFilter matches repositories with a clean worktree.
type cleanFilter struct{}

func (cleanFilter) Matches(path string) (bool, error) {
	return isClean(path)
}

// stashFilter matches repositories with at least one stash.
type stashFilter struct{}

func (stashFilter) Matches(path string) (bool, error) {
	count, err := getStashCount(path)
	return count > 0, err
}

// hasRemoteFilter matches repositories with at least one remote.
type hasRemoteFilter struct{}

func (hasRemoteFilter) Matches(path string) (bool, error) {
	urls, err := getRemoteURLs(path)
	return len(urls) > 0, err
}

// noRemoteFilter matches repositories with no remotes.
type noRemoteFilter struct{}

func (noRemoteFilter) Matches(path string) (bool, error) {
	urls, err := getRemoteURLs(path)
	return len(urls) == 0, err
}

// remoteContainsFilter matches repositories with a remote URL containing substring.
type remoteContainsFilter struct {
	substring string
}

func (f remoteContainsFilter) Matches(path string) (bool, error) {
	urls, err := getRemoteURLs(path)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(urls, func(u string) bool {
		return strings.Contains(u, f.substring)
	}), nil
}

// remoteHostFilter matches repositories with a remote on host.
type remoteHostFilter struct {
	host string
}

func (f remoteHostFilter) Matches(path string) (bool, error) {
	urls, err := getRemoteURLs(path)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(urls, func(u string) bool {
		return remoteHost(u) == f.host
	}), nil
}

// remoteNameFilter matches repositories with a remote named name.
type remoteNameFilter struct {
	name string
}

func (f remoteNameFilter) Matches(path string) (bool, error) {
	names, err := getRemoteNames(path)
	if err != nil {
		return false, err
	}
	return slices.Contains(names, f.name), nil
}

// nameContainsFilter matches repositories whose directory name contains substring.
type nameContainsFilter struct {
	substring string
}

func (f nameContainsFilter) Matches(path string) (bool, error) {
	return strings.Contains(filepath.Base(path), f.substring), nil
}

// nameStartsFilter matches repositories whose directory name starts with prefix.
type nameStartsFilter struct {
	prefix string
}

func (f nameStartsFilter) Matches(path string) (bool, error) {
	return strings.HasPrefix(filepath.Base(path), f.prefix), nil
}

// nameEndsFilter matches repositories whose directory name ends with suffix.
type nameEndsFilter struct {
	suffix string
}

func (f nameEndsFilter) Matches(path string) (bool, error) {
	return strings.HasSuffix(filepath.Base(path), f.suffix), nil
}

// buildFilters assembles the filters selected by the command line options.
func buildFilters(opts *options) []Filter {
	var filters []Filter

	if opts.branch != "" {
		filters = append(filters, branchFilter{branch: opts.branch})
	}
	if opts.dirty {
		filters = append(filters, dirtyFilter{})
	}
	if opts.clean {
		filters = append(filters, cleanFilter{})
	}
	if opts.stash {
		filters = append(filters, stashFilter{})
	}
	if opts.remote {
		filters = append(filters, hasRemoteFilter{})
	}
	if opts.noRemote {
		filters = append(filters, noRemoteFilter{})
	}
	if opts.remoteContains != "" {
		filters = append(filters, remoteContainsFilter{substring: opts.remoteContains})
	}
	if opts.remoteHost != "" {
		filters = append(filters, remoteHostFilter{host: opts.remoteHost})
	}
	if opts.remoteName != "" {
		filters = append(filters, remoteNameFilter{name: opts.remoteName})
	}
	if opts.nameContains != "" {
		filters = append(filters, nameContainsFilter{substring: opts.nameContains})
	}
	if opts.nameStarts != "" {
		filters = append(filters, nameStartsFilter{prefix: opts.nameStarts})
	}
	if opts.nameEnds != "" {
		filters = append(filters, nameEndsFilter{suffix: opts.nameEnds})
	}

	return filters
}
