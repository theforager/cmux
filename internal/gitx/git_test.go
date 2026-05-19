package gitx

import "testing"

func TestWorktreeForBranchFromPorcelain(t *testing.T) {
	out := `worktree /Users/dev/dev/cmux
HEAD abc123
branch refs/heads/main

worktree /Users/dev/dev/worktrees/REB-135-test-out-rivet-package-on-fe-prod
HEAD def456
branch refs/heads/0xforager/reb-135-test-out-rivet-package-on-fe-prod
`

	got, ok := worktreeForBranchFromPorcelain(out, "0xforager/reb-135-test-out-rivet-package-on-fe-prod")
	if !ok {
		t.Fatal("expected branch worktree to be found")
	}
	want := "/Users/dev/dev/worktrees/REB-135-test-out-rivet-package-on-fe-prod"
	if got != want {
		t.Fatalf("worktree = %q, want %q", got, want)
	}
}

func TestWorktreeForBranchFromPorcelainMissing(t *testing.T) {
	out := `worktree /repo
HEAD abc123
branch refs/heads/main
`

	if got, ok := worktreeForBranchFromPorcelain(out, "feature/missing"); ok {
		t.Fatalf("expected no match, got %q", got)
	}
}

func TestWorktreeEntriesFromPorcelainStripsRefsHeads(t *testing.T) {
	out := `worktree /repo
HEAD abc123
branch refs/heads/main

worktree /repo/wt
HEAD def456
branch refs/heads/feature/test
`
	entries := worktreeEntriesFromPorcelain(out)
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[1].Path != "/repo/wt" || entries[1].Branch != "feature/test" {
		t.Fatalf("entry = %+v, want path /repo/wt branch feature/test", entries[1])
	}
}

func TestWorktreeForBranchFromPorcelainHandlesTrailingRecord(t *testing.T) {
	out := `worktree /repo
HEAD abc123
branch refs/heads/main`
	got, ok := worktreeForBranchFromPorcelain(out, "main")
	if !ok || got != "/repo" {
		t.Fatalf("worktree = %q/%v, want /repo true", got, ok)
	}
}

func TestRemoteBranchFromListPrefersOrigin(t *testing.T) {
	out := `upstream/feature/test
origin/HEAD
origin/feature/test
`
	got, ok := remoteBranchFromList(out, "feature/test")
	if !ok || got != "origin/feature/test" {
		t.Fatalf("remote = %q/%v, want origin/feature/test true", got, ok)
	}
}

func TestRemoteBranchFromListFallsBackToOtherRemote(t *testing.T) {
	out := `upstream/feature/test
origin/other
`
	got, ok := remoteBranchFromList(out, "feature/test")
	if !ok || got != "upstream/feature/test" {
		t.Fatalf("remote = %q/%v, want upstream/feature/test true", got, ok)
	}
}
