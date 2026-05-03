package gitx

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/theforager/cmux/internal/format"
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
	root := filepath.Join(filepath.Dir(repo), "worktrees")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", "", "", err
	}
	worktree = filepath.Join(root, identifier+"-"+format.Slug(title))
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

func branchExists(repo, branch string) bool {
	_, err := process.RunDir(repo, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}
