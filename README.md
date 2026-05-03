# cmux

Terminal-first workbench for long-lived coding-agent sessions.

cmux manages Claude/Codex sessions in tmux and adds structured
agent orchestration:

- Linear issue-backed sessions
- task-backed and scratch sessions
- git worktree isolation
- durable local session registry
- `.agent/RUNBOOK.md` per real workspace
- Linear status/comment sync
- Bubble Tea based session dashboard

The original Bash implementation is preserved at `legacy/cmux.bash` and can be
run with `cmux legacy ...`.

## Installation

```bash
git clone <repo-url> cmux
cd cmux
./install.sh
```

The installer builds the Go CLI and symlinks `cmux` into `~/bin`.

For development:

```bash
go build -o dist/cmux-go ./cmd/cmux
./cmux help
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
R              refresh
?              help
q / esc        quit
```

Inside any cmux tmux session, press `prefix + g` to open the popup switcher.
By default tmux prefix is `Ctrl-b`, so press `Ctrl-b` then `g`.

## Agent Orchestration

```bash
cmux agent start REB-123
cmux agent start --title "Investigate Vercel build failure" --worktree
cmux agent start REB-123 --prepare
cmux agent scratch --title "Explore SDK publishing bug"
cmux agent list
cmux agent open REB-123
cmux agent status REB-123 blocked "Need API shape decision"
cmux agent review REB-123 "Ready for review"
cmux agent done REB-123
```

Session types:

- `issue-backed`: Linear issue, branch/worktree, runbook, Linear sync.
- `task-backed`: manually named goal, optional worktree.
- `scratch`: quick current-directory session.

For issue-backed and task-backed workspaces, cmux writes:

```text
.agent/RUNBOOK.md
.agent/cmux.json
```

The initial agent prompt tells the agent to keep the runbook updated and to set
cmux statuses when blocked, failed, or ready for review.

## State And Config

Local session state is stored outside the repo:

```text
~/.local/state/cmux/agents/*.json
```

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
cmux legacy ...
cmux help
```
