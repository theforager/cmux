package agent

import (
	"testing"
	"time"

	"github.com/theforager/cmux/internal/types"
)

func TestClassifyRuntimeStatusDeadPaneCrashes(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	status := ClassifyRuntimeStatus(types.StatusRunning, types.RuntimeData{TmuxAlive: true, PaneDead: true}, now)
	if status != types.StatusCrashed {
		t.Fatalf("status = %s, want %s", status, types.StatusCrashed)
	}
}

func TestClassifyRuntimeStatusPreservesManualBlocked(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	last := now.Add(-24 * time.Hour).Format(time.RFC3339)
	status := ClassifyRuntimeStatus(types.StatusBlocked, types.RuntimeData{TmuxAlive: true, LastActivityAt: last, CurrentCommand: "zsh"}, now)
	if status != types.StatusBlocked {
		t.Fatalf("status = %s, want %s", status, types.StatusBlocked)
	}
}

func TestClassifyRuntimeStatusPreservesPreparedIdleWithoutTmux(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	status := ClassifyRuntimeStatus(types.StatusIdle, types.RuntimeData{TmuxAlive: false}, now)
	if status != types.StatusIdle {
		t.Fatalf("status = %s, want %s", status, types.StatusIdle)
	}
}

func TestClassifyRuntimeStatusDoesNotTreatShellPromptAsWaiting(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	last := now.Add(-20 * time.Minute).Format(time.RFC3339)
	status := ClassifyRuntimeStatus(types.StatusRunning, types.RuntimeData{TmuxAlive: true, LastActivityAt: last, CurrentCommand: "zsh", Preview: "repo $"}, now)
	if status != types.StatusIdle {
		t.Fatalf("status = %s, want %s", status, types.StatusIdle)
	}
}

func TestClassifyRuntimeStatusShellAfterAgentExitIsIdle(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	status := ClassifyRuntimeStatus(types.StatusRunning, types.RuntimeData{TmuxAlive: true, CurrentCommand: "fish"}, now)
	if status != types.StatusIdle {
		t.Fatalf("status = %s, want %s", status, types.StatusIdle)
	}
}

func TestClassifyRuntimeStatusWaitingForApprovalPrompt(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	last := now.Add(-20 * time.Minute).Format(time.RFC3339)
	status := ClassifyRuntimeStatus(types.StatusRunning, types.RuntimeData{TmuxAlive: true, LastActivityAt: last, CurrentCommand: "codex", Preview: "Permission required: allow command? yes/no"}, now)
	if status != types.StatusWaiting {
		t.Fatalf("status = %s, want %s", status, types.StatusWaiting)
	}
}

func TestClassifyRuntimeStatusDoesNotMarkStaleForInactivity(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	last := now.Add(-48 * time.Hour).Format(time.RFC3339)
	status := ClassifyRuntimeStatus(types.StatusRunning, types.RuntimeData{TmuxAlive: true, LastActivityAt: last, CurrentCommand: "claude"}, now)
	if status != types.StatusRunning {
		t.Fatalf("status = %s, want %s", status, types.StatusRunning)
	}
}
