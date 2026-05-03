package gitx

import (
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

func EnsureWorktree(cwd, identifier, title, branchName string) (repo, branch, worktree string, err error) {
	repo, err = Root(cwd)
	if err != nil {
		return "", "", "", err
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
