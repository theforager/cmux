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
