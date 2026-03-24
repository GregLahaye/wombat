// Package source manages git operations on source repositories and
// auto-discovers skills and agents within them.
package source

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Clone performs a shallow clone of url into dest (via temp dir + rename).
func Clone(url, dest string) error {
	if strings.HasPrefix(url, "-") {
		return fmt.Errorf("invalid git URL: %s", url)
	}
	if dest == "" {
		return fmt.Errorf("empty destination path")
	}
	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("creating parent: %w", err)
	}

	tmp, err := os.MkdirTemp(parent, ".wombat-clone-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}

	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(tmp)
		}
	}()

	if err := git("", "clone", "--depth", "1", url, tmp); err != nil {
		return fmt.Errorf("cloning %s: %w", url, err)
	}

	// Remove dest only if it exists (expected for re-clones).
	if _, statErr := os.Stat(dest); statErr == nil {
		os.RemoveAll(dest)
	}
	if err := os.Rename(tmp, dest); err != nil {
		return fmt.Errorf("moving clone: %w", err)
	}

	// Best-effort: set origin/HEAD for DefaultBranch detection.
	_ = git(dest, "remote", "set-head", "origin", "--auto")

	ok = true
	return nil
}

// Update fetches and resets to the latest commit on the default branch.
func Update(repoPath string) error {
	branch, err := DefaultBranch(repoPath)
	if err != nil {
		return fmt.Errorf("detecting default branch: %w", err)
	}
	if err := git(repoPath, "fetch", "--depth", "1", "origin", branch); err != nil {
		return fmt.Errorf("fetching origin/%s: %w", branch, err)
	}
	if err := git(repoPath, "reset", "--hard", "FETCH_HEAD"); err != nil {
		return fmt.Errorf("resetting to FETCH_HEAD: %w", err)
	}
	return nil
}

// HasUpdates checks whether the remote has newer commits than local HEAD.
func HasUpdates(repoPath string) (bool, error) {
	branch, err := DefaultBranch(repoPath)
	if err != nil {
		return false, err
	}
	local, err := gitRun(repoPath, "rev-parse", "HEAD")
	if err != nil {
		return false, err
	}
	remote, err := gitRun(repoPath, "ls-remote", "origin", "refs/heads/"+branch)
	if err != nil {
		return false, err
	}
	fields := strings.Fields(remote)
	if len(fields) == 0 {
		return false, fmt.Errorf("no remote ref for branch %s", branch)
	}
	return strings.TrimSpace(local) != fields[0], nil
}

// DefaultBranch returns the default branch name for the repo.
func DefaultBranch(repoPath string) (string, error) {
	if out, err := gitRun(repoPath, "symbolic-ref", "refs/remotes/origin/HEAD"); err == nil {
		ref := strings.TrimSpace(out)
		if after, ok := strings.CutPrefix(ref, "refs/remotes/origin/"); ok {
			return after, nil
		}
	}
	out, err := gitRun(repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("detecting default branch: %w", err)
	}
	branch := strings.TrimSpace(out)
	if branch == "" || branch == "HEAD" {
		return "", fmt.Errorf("could not determine default branch")
	}
	return branch, nil
}

// RemoteURL returns the origin URL for the repo at repoPath.
func RemoteURL(repoPath string) (string, error) {
	out, err := gitRun(repoPath, "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// RepoRoot returns the top-level directory of the git repo containing path.
func RepoRoot(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	dir := path
	if !info.IsDir() {
		dir = filepath.Dir(path)
	}
	out, err := gitRun(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func git(dir string, args ...string) error {
	_, err := gitRun(dir, args...)
	return err
}

func gitRun(dir string, args ...string) (string, error) {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(stderr.String()), err)
	}
	return stdout.String(), nil
}
