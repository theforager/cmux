# cmux Agent Orchestration Handoff

This file is context for a follow-on coding agent continuing the cmux work.

## Product Context

cmux started as a tmux session switcher for long-lived coding-agent sessions. The product direction is to make it a terminal-first agent workbench:

```text
Linear issue / task / scratch goal
  -> workspace / git worktree
  -> cmux tmux session
  -> agent process
  -> durable state + runbook
  -> status / review / Linear sync
```

The golden path is Linear issue-backed work, but scratch and task-backed sessions must remain easy. The tool should not become a heavy agent framework. It should make parallel coding agents legible, resumable, isolated, and connected to real issue goals.

## Current Implementation

The project is now a Go CLI using Cobra and Bubble Tea. The TypeScript rewrite was removed. Installation builds a real binary to `~/bin/cmux`.

Important commits:

- `ddcead5 Rewrite cmux orchestration in TypeScript`
- `954658f Port cmux runtime to Go and Bubble Tea`
- `fb3aa58 Simplify Go install and selector actions`
- `26db453 Improve cmux agent session workflow`
- `972568f Add session detail view and workspace actions`

Key packages:

- `cmd/cmux/main.go`: Cobra CLI, command wiring.
- `internal/tui/selector.go`: Bubble Tea session selector/dashboard.
- `internal/agent/agent.go`: structured agent start/status/kill logic.
- `internal/state/state.go`: session metadata persistence.
- `internal/home/home.go`: unified cmux home layout.
- `internal/gitx/git.go`: repo and worktree management.
- `internal/linear/linear.go`: Linear GraphQL issue/comment sync.
- `internal/runbook/runbook.go`: runbook section parser.
- `internal/tmux/tmux.go`: tmux creation, attach/switch, popup binding.

## State Layout

cmux intentionally stores its own data in one place, not in repo-local config:

```text
~/.cmux/
  config.json          # planned, limited user-facing settings only
  sessions/
    <session-id>/
      session.json
      RUNBOOK.md
  worktrees/
    <repo-name>/
      <issue-or-task-slug>/
  hooks/               # planned status-change hooks
```

Override with:

```sh
CMUX_HOME=/path/to/cmux-home
```

cmux should avoid writing `.agent`, `.cmux`, or other repo-local metadata unless a future design explicitly opts into it.

## Current User Flows

Main selector:

```sh
cmux
cmux switch
```

Core keys:

- `enter`: open selected session.
- `i`: detail view for selected session.
- `w`: open a plain shell tmux session in selected workspace/worktree.
- `p`: show selected workspace path inline.
- `n`: create scratch session.
- `a`: start Linear issue-backed or task-backed session.
- `s`: set structured session status.
- `/`: filter.
- `?`: help.

Agent creation from the TUI now uses a highlighted picker:

- Claude
- Codex
- Other custom command

Numeric shortcuts `1/2/3` still work.

CLI examples:

```sh
cmux agent start REB-123
cmux agent start --title "Investigate Vercel build failure" --worktree
cmux agent start REB-123 --prepare
cmux agent scratch --title "Explore SDK publishing bug"
cmux agent list
cmux agent open REB-123
cmux agent path REB-123
cmux agent status REB-123 blocked "Need API shape decision"
cmux agent review REB-123 "Ready for review"
cmux agent done REB-123
cmux doctor
```

Linear requires:

```sh
export LINEAR_API_KEY=lin_api_xxx
```

Be aware that tmux popups may inherit the tmux server environment, not the current shell environment. If `LINEAR_API_KEY` was exported after tmux started, the TUI can report that it is not set in the cmux process.

## Implemented Features

- Go/Bubble Tea dashboard.
- tmux popup switcher via `prefix + g`.
- Linear issue-backed session start.
- task-backed and scratch sessions.
- default issue worktrees under `~/.cmux/worktrees`.
- per-session JSON state and cmux-owned `RUNBOOK.md`.
- initial agent prompt that instructs agents to update the runbook and set cmux status.
- Linear comment sync with status, branch, workspace, runbook path, and parsed runbook sections.
- TUI status override menu.
- TUI session detail view.
- TUI workspace path and workspace shell actions.
- `cmux agent path <id>`.
- clearer Linear HTTP/API errors.
- safer install path to `~/bin/cmux`.

## Current Uncommitted Work

There is an uncommitted safety fix for git worktree conflicts:

- `internal/gitx/git.go`
- `internal/gitx/git_test.go`

Context: attempting to start a Linear issue whose Linear-provided branch is already checked out in an older external worktree caused Git to fail:

```text
fatal: '<branch>' is already checked out at '<existing-worktree>'
```

The initial idea was to silently reuse that existing worktree, but that is risky because another agent or human could be using it. The current uncommitted behavior detects this and fails early with a clearer cmux error instead of auto-adopting the worktree.

This should likely be committed after review. A future explicit adoption flow should check whether another cmux session already uses that workspace before launching anything there.

## Design Principles To Preserve

- Linear issue-backed sessions are first-class, not mandatory.
- Scratch sessions must stay quick and low ceremony.
- Real issue/task work should prefer isolated worktrees.
- Do not silently attach new agents to existing worktrees.
- Keep orchestration logic usable from CLI, not only from the TUI.
- Avoid repo-local cmux config/state.
- Keep user config minimal and high-value.
- Prefer explicit actions over background magic until the state model is stronger.
- Surface errors in the TUI. Do not let create actions look like no-ops.

## Outstanding Product Work

### 1. Explicit Existing Worktree Adoption

Add a deliberate flow for the case where a branch already has a worktree:

```sh
cmux agent start REB-135 --adopt-worktree /path/to/worktree
```

or a TUI confirmation:

```text
Branch is already checked out at:
/Users/dev/dev/worktrees/REB-135-test-out-rivet-package-on-fe-prod

Open shell / adopt / cancel
```

Before adopting, check:

- Does any cmux session already use that workspace?
- Is there an active tmux session for that session id?
- Is the worktree dirty?
- Is the user explicitly asking for an agent or only a workspace shell?

Do not auto-adopt by default.

### 2. Better Linear Sync Surface

Current sync is mostly implicit. Needed:

- TUI action to sync selected session to Linear now.
- Store last sync time and last sync error in `session.json`.
- Show sync status/errors in detail view.
- Optional CLI command:

```sh
cmux agent sync REB-123
```

### 3. Daemon / Watch Mode

The user wants lightweight automation eventually:

- detect crashed/dead tmux sessions.
- mark stale sessions.
- run hooks on status transitions.
- optionally poll Linear for issue state changes and prepare work.
- notify when human intervention is needed.

Keep this lightweight. A first version could be:

```sh
cmux daemon
cmux agent scan
```

### 4. Notifications And Hooks

Proposed hook:

```text
~/.cmux/hooks/on-status-change
```

Environment:

```sh
CMUX_SESSION_ID=REB-123
CMUX_OLD_STATUS=running
CMUX_NEW_STATUS=ready_for_review
CMUX_LINEAR_ISSUE_ID=REB-123
CMUX_WORKTREE=/path/to/worktree
```

### 5. Config

Config should live at `~/.cmux/config.json`, but only include settings that users are likely to change:

- default agent command.
- worktree root override.
- optional editor/open command.
- maybe Linear team/project defaults later.

Avoid scattering config in XDG, repo-local files, and session folders.

### 6. Open IDE / Editor Action

The TUI currently has `w` for a workspace shell and `p` for path. Later add an editor action:

- open selected workspace in Cursor/VS Code/terminal editor.
- should be configurable.
- should not require GUI assumptions for remote/mobile terminal users.

### 7. Status Model Automation

Current manual status override is intentionally simple. Future automation can detect:

- dead pane -> `crashed`.
- prompt waiting -> `waiting_for_input`.
- runbook markers -> `blocked` / `ready_for_review`.
- test failures from explicit agent commands, not arbitrary terminal text if possible.

Manual TUI status should remain as an override.

## Known Risks / Bugs

- tmux server environment can miss newly exported `LINEAR_API_KEY`.
- Shelling into workspaces should not accidentally spawn duplicate confusing sessions.
- Worktree lifecycle cleanup is not implemented.
- No migration exists for old state layouts, if users have them.
- Linear sync failures are currently not persisted.
- Existing external worktree conflicts need a first-class adoption UX.
- There are no tests for most TUI flows; logic should be kept simple and extracted when possible.

## Verification Commands

Use a sandbox-writable Go cache when needed:

```sh
env GOCACHE=/tmp/cmux-go-build-cache go test ./...
env GOCACHE=/tmp/cmux-go-build-cache go build -o /tmp/cmux-preview ./cmd/cmux
git diff --check
./install.sh
cmux --version
cmux doctor
```

The `/tmp` paths above are verification-only, not product architecture.

## Recommended Next Task

Finish and commit the existing-worktree guard, then add an explicit adoption design:

1. Commit the guard after checking the error wording.
2. Add `cmux agent start --adopt-worktree <path>` or a separate `cmux agent adopt <issue> <path>`.
3. Store the adopted workspace in normal session state.
4. Add TUI affordance when a branch conflict is detected.
5. Ensure adoption refuses if another cmux session already points at the workspace unless explicitly forced.
