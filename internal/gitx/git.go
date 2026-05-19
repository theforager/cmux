package gitx

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/theforager/cmux/internal/format"
	"github.com/theforager/cmux/internal/home"
	"github.com/theforager/cmux/internal/process"
)

func Root(cwd string) (string, error) {
	out, err := process.RunDir(cwd, "git", "rev-parse", "--show-toplevel")
	return strings.TrimSpace(out), err
}

func StatusSummary(cwd string) (bool, string) {
	out, err := process.RunDir(cwd, "git", "status", "--short")
	if err != nil {
		return false, ""
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return false, "clean"
	}
	lines := strings.Split(out, "\n")
	summary := strings.Join(lines, "\n")
	if len(lines) > 8 {
		summary = strings.Join(lines[:8], "\n") + "\n..."
	}
	return true, summary
}

func Head(cwd string) (string, error) {
	out, err := process.RunDir(cwd, "git", "rev-parse", "HEAD")
	return strings.TrimSpace(out), err
}

func DiffHash(cwd string) (string, error) {
	out, err := process.RunDir(cwd, "git", "diff", "--binary")
	if err != nil {
		return "", err
	}
	staged, err := process.RunDir(cwd, "git", "diff", "--cached", "--binary")
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(out + "\n" + staged))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func WorktreeListed(repo, worktree string) bool {
	out, err := process.RunDir(repo, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return false
	}
	for _, entry := range worktreeEntriesFromPorcelain(out) {
		if samePath(entry.Path, worktree) {
			return true
		}
	}
	return false
}

func RemoveWorktree(repo, worktree string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, worktree)
	_, err := process.RunDir(repo, "git", args...)
	return err
}

func ResetHardClean(worktree string) error {
	if _, err := process.RunDir(worktree, "git", "reset", "--hard"); err != nil {
		return err
	}
	_, err := process.RunDir(worktree, "git", "clean", "-fd")
	return err
}

func EnsureWorktree(cwd, identifier, title, branchName string) (repo, branch, worktree string, err error) {
	repo, err = Root(cwd)
	if err != nil {
		return "", "", "", fmt.Errorf("not a git repository: %s; choose a repo for Linear work or use Scratch session for non-git folders", cwd)
	}
	branch = branchName
	if branch == "" {
		branch = "agent/" + identifier + "-" + format.Slug(title)
	}
	worktree = home.WorktreePath(repo, identifier, title)
	if err := os.MkdirAll(filepath.Dir(worktree), 0o755); err != nil {
		return "", "", "", err
	}
	if info, err := os.Stat(worktree); err == nil {
		if !info.IsDir() {
			return "", "", "", fmt.Errorf("worktree path exists and is not a directory: %s", worktree)
		}
		if err := validateExistingWorktree(repo, worktree, branch); err != nil {
			return "", "", "", err
		}
		return repo, branch, worktree, nil
	} else if !os.IsNotExist(err) {
		return "", "", "", err
	}
	if existing, ok := worktreeForBranch(repo, branch); ok {
		return "", "", "", fmt.Errorf("branch %q is already checked out at %s; cmux will not automatically attach a new agent to an existing worktree", branch, existing)
	}
	if branchExists(repo, branch) {
		_, err = process.RunDir(repo, "git", "worktree", "add", worktree, branch)
	} else if remote, ok := remoteBranch(repo, branch); ok {
		_, err = process.RunDir(repo, "git", "worktree", "add", "-b", branch, worktree, remote)
	} else {
		_, err = process.RunDir(repo, "git", "worktree", "add", "-b", branch, worktree)
	}
	return repo, branch, worktree, err
}

func branchExists(repo, branch string) bool {
	_, err := process.RunDir(repo, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

func remoteBranch(repo, branch string) (string, bool) {
	out, err := process.RunDir(repo, "git", "for-each-ref", "--format=%(refname:short)", "refs/remotes")
	if err != nil {
		return "", false
	}
	return remoteBranchFromList(out, branch)
}

func remoteBranchFromList(out, branch string) (string, bool) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "", false
	}
	preferred := "origin/" + branch
	var fallback string
	for _, line := range strings.Split(out, "\n") {
		ref := strings.TrimSpace(line)
		if ref == "" || strings.HasSuffix(ref, "/HEAD") {
			continue
		}
		if ref == preferred {
			return ref, true
		}
		if fallback == "" && strings.HasSuffix(ref, "/"+branch) {
			fallback = ref
		}
	}
	if fallback != "" {
		return fallback, true
	}
	return "", false
}

func validateExistingWorktree(repo, worktree, branch string) error {
	out, err := process.RunDir(repo, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return err
	}
	for _, entry := range worktreeEntriesFromPorcelain(out) {
		if !samePath(entry.Path, worktree) {
			continue
		}
		if entry.Branch == "" {
			return fmt.Errorf("worktree path already exists but is not on branch %q: %s", branch, worktree)
		}
		if entry.Branch != branch {
			return fmt.Errorf("worktree path %s is checked out on branch %q, expected %q", worktree, entry.Branch, branch)
		}
		return nil
	}
	return fmt.Errorf("worktree path already exists but git does not list it for this repository: %s", worktree)
}

func worktreeForBranch(repo, branch string) (string, bool) {
	out, err := process.RunDir(repo, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return "", false
	}
	return worktreeForBranchFromPorcelain(out, branch)
}

func worktreeForBranchFromPorcelain(out, branch string) (string, bool) {
	for _, entry := range worktreeEntriesFromPorcelain(out) {
		if entry.Branch == branch && entry.Path != "" {
			return entry.Path, true
		}
	}
	return "", false
}

type worktreeEntry struct {
	Path   string
	Branch string
}

func worktreeEntriesFromPorcelain(out string) []worktreeEntry {
	var entries []worktreeEntry
	current := worktreeEntry{}
	flush := func() {
		if current.Path != "" {
			entries = append(entries, current)
		}
		current = worktreeEntry{}
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			current.Path = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimSpace(strings.TrimPrefix(line, "branch "))
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case strings.TrimSpace(line) == "":
			flush()
		}
	}
	flush()
	return entries
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}
