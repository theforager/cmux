# cmux

Terminal-first workbench for long-lived coding-agent sessions.

cmux still manages Claude/Codex sessions in tmux, but v2 adds structured
agent orchestration:

- Linear issue-backed sessions
- task-backed and scratch sessions
- git worktree isolation
- durable local session registry
- `.agent/RUNBOOK.md` per real workspace
- status transitions and hooks
- Linear status/comment sync
- lightweight Linear polling daemon
- Ink-based session dashboard

The original Bash implementation is preserved at `legacy/cmux.bash` and can be
run with `cmux legacy ...`.

## Installation

```bash
git clone <repo-url> cmux
cd cmux
./install.sh
```

The installer runs `npm install`, builds the TypeScript CLI, and symlinks
`cmux` into `~/bin`.

For development:

```bash
npm install
npm run build
./cmux help
```

## Basic Usage

```bash
cmux                         # Interactive Ink selector
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
b              mark structured agent blocked
f              mark structured agent tests_failed
v              mark structured agent ready_for_review
x              mark structured agent done
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
cmux agent promote scratch-20260502-1 --issue REB-123
cmux agent sync REB-123
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

Config lives at:

```text
~/.config/cmux/config.json
```

Example:

```json
{
  "defaultAgent": "claude",
  "worktreeRoot": "../worktrees",
  "linear": {
    "apiKeyEnv": "LINEAR_API_KEY",
    "moveToStateOnStart": "In Progress",
    "managedComment": true,
    "syncStatus": true,
    "statusMap": {
      "running": "In Progress",
      "blocked": "Blocked",
      "tests_failed": "Blocked",
      "ready_for_review": "In Review",
      "done": "Done"
    },
    "autoStart": {
      "enabled": true,
      "mode": "prepare",
      "intervalSeconds": 60,
      "teamKeys": ["REB"],
      "states": ["Ready for Agent"],
      "labels": ["cmux"],
      "maxConcurrent": 3
    }
  }
}
```

Set your Linear token with:

```bash
export LINEAR_API_KEY=lin_api_xxx
```

## Linear Daemon

```bash
cmux daemon --once     # Poll once and exit
cmux daemon            # Poll continuously
```

The daemon reads `linear.autoStart` from config. In `prepare` mode it creates
the registry/worktree/runbook without launching an agent. In `start` mode it
also launches the configured agent in tmux.

## Status Hooks

cmux runs this hook when a structured session changes status:

```text
~/.config/cmux/hooks/on-status-change
```

Environment:

```text
CMUX_SESSION_ID
CMUX_OLD_STATUS
CMUX_NEW_STATUS
CMUX_LINEAR_ISSUE_ID
CMUX_WORKTREE
```

Use this for macOS notifications, Slack, Linear comments, or other local
automation.

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
cmux ssh <host|alias>
cmux agent ...
cmux daemon [--once]
cmux legacy ...
cmux help
```
