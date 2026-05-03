# cmux

Terminal-first workbench for long-lived coding-agent sessions.

cmux manages Claude/Codex sessions in tmux and adds structured
agent orchestration:

- Linear issue-backed sessions
- task-backed and scratch sessions
- git worktree isolation
- durable local session registry
- cmux-owned runbooks per real workspace
- Linear status/comment sync
- Bubble Tea based session dashboard

## Installation

```bash
git clone <repo-url> cmux
cd cmux
./install.sh
```

The installer builds the Go CLI and symlinks `cmux` into `~/bin`.

For development:

```bash
go run ./cmd/cmux help
go build -o ./cmux-dev ./cmd/cmux
```

## Basic Usage

```bash
cmux                         # Interactive Bubble Tea selector
cmux new ~/projects/app       # Create a plain tmux agent session
cmux new -a codex .           # Use Codex instead of Claude
cmux list                     # List tmux sessions
cmux attach app               # Attach/switch to a session
cmux switch                   # Open selector from inside tmux
```

## Dashboard Keys

The selector is still optimized for fast session switching:

```text
↑↓ / j k       navigate
home / end     first / last
pgup / pgdn    move by 5
1-9 digits     jump to row number
enter          open selected session
/              filter
n              create scratch session
a              start Linear issue-backed or task-backed agent
r              rename selected tmux session
t              set selected title
d              delete selected session
s              set structured agent status
R              refresh
?              help
q / esc        quit
```

Inside any cmux tmux session, press `prefix + g` to open the popup switcher.
By default tmux prefix is `Ctrl-b`, so press `Ctrl-b` then `g`.

When creating sessions from the dashboard, cmux asks which agent to run:

```text
1 claude
2 codex
3 other
```

## Agent Orchestration

```bash
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
```

Session types:

- `issue-backed`: Linear issue, branch/worktree, runbook, Linear sync.
- `task-backed`: manually named goal, optional worktree.
- `scratch`: quick current-directory session.

cmux keeps session metadata and runbooks under its own home directory:

```text
~/.cmux/sessions/<session-id>/session.json
~/.cmux/sessions/<session-id>/RUNBOOK.md
```

The initial agent prompt tells the agent to keep the runbook updated and to set
cmux statuses when blocked, failed, or ready for review.

## Home Directory

cmux stores its own data in one home directory:

```text
~/.cmux/
  config.json
  sessions/
  worktrees/
  hooks/
```

Override it with `CMUX_HOME=/path/to/cmux-home`.

Set your Linear token with:

```bash
export LINEAR_API_KEY=lin_api_xxx
```

## Commands

```text
cmux
cmux new [--agent|-a <cmd>] [--title|-t <text>] [--mobile|-m] [--no-attach] [path]
cmux list
cmux attach <name>
cmux switch
cmux kill <name>
cmux rename <name>
cmux title <text>
cmux info
cmux debug
cmux doctor
cmux agent ...
cmux help
```
