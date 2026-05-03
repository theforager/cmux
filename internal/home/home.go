package home

import (
	"os"
	"path/filepath"

	"github.com/theforager/cmux/internal/format"
)

func Dir() string {
	if v := os.Getenv("CMUX_HOME"); v != "" {
		return expand(v)
	}
	userHome, _ := os.UserHomeDir()
	return filepath.Join(userHome, ".cmux")
}

func ConfigPath() string {
	return filepath.Join(Dir(), "config.json")
}

func SessionsDir() string {
	return filepath.Join(Dir(), "sessions")
}

func SessionDir(id string) string {
	return filepath.Join(SessionsDir(), id)
}

func SessionPath(id string) string {
	return filepath.Join(SessionDir(id), "session.json")
}

func RunbookPath(id string) string {
	return filepath.Join(SessionDir(id), "RUNBOOK.md")
}

func WorktreesDir() string {
	return filepath.Join(Dir(), "worktrees")
}

func WorktreePath(repoPath, identifier, title string) string {
	repoName := format.Slug(filepath.Base(repoPath))
	return filepath.Join(WorktreesDir(), repoName, identifier+"-"+format.Slug(title))
}

func HooksDir() string {
	return filepath.Join(Dir(), "hooks")
}

func expand(path string) string {
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if len(path) > 2 && path[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
