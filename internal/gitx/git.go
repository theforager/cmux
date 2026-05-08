package gitx

import (
	"fmt"
	"os"
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

func WorktreeListed(repo, worktree string) bool {
	out, err := process.RunDir(repo, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(strings.TrimPrefix(line, "worktree ")) == worktree && strings.HasPrefix(line, "worktree ") {
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
	if err := os.MkdirAll(filepathDir(worktree), 0o755); err != nil {
		return "", "", "", err
	}
	if _, err := os.Stat(worktree); err == nil {
		return repo, branch, worktree, nil
	}
	if existing, ok := worktreeForBranch(repo, branch); ok {
		return "", "", "", fmt.Errorf("branch %q is already checked out at %s; cmux will not automatically attach a new agent to an existing worktree", branch, existing)
	}
	if branchExists(repo, branch) {
		_, err = process.RunDir(repo, "git", "worktree", "add", worktree, branch)
	} else {
		_, err = process.RunDir(repo, "git", "worktree", "add", "-b", branch, worktree)
	}
	return repo, branch, worktree, err
}

func filepathDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

func branchExists(repo, branch string) bool {
	_, err := process.RunDir(repo, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

func worktreeForBranch(repo, branch string) (string, bool) {
	out, err := process.RunDir(repo, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return "", false
	}
	return worktreeForBranchFromPorcelain(out, branch)
}

func worktreeForBranchFromPorcelain(out, branch string) (string, bool) {
	want := "refs/heads/" + branch
	currentPath := ""
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			currentPath = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimSpace(strings.TrimPrefix(line, "branch "))
			if ref == want && currentPath != "" {
				return currentPath, true
			}
		case strings.TrimSpace(line) == "":
			currentPath = ""
		}
	}
	return "", false
}
