# cmux

Terminal-first operator console for long-lived coding-agent work.

cmux runs Claude, Codex, or another agent in tmux, tracks each real work item as
a structured session, isolates issue work in git worktrees, and keeps Linear in
sync with the parts of the workflow that should outlive the terminal.

## What It Manages

- Active Claude/Codex/tmux sessions
- Linear issue-backed agent sessions
- Task-backed and scratch sessions
- Per-session git worktrees
- Local runbooks for durable agent context
- Linear worklist presets and queue browsing
- Session lifecycle actions: scope, review, done, close, forget, reset

Most day-to-day use starts with:

```bash
cmux
```

The CLI subcommands are still available for scripting, but the TUI is the
primary workflow.

## Install

```bash
git clone <repo-url> cmux
cd cmux
./install.sh
```

The installer builds the Go CLI into `~/bin/cmux`.

For development:

```bash
go run ./cmd/cmux help
go build -o ./cmux-dev ./cmd/cmux
```

## Daily Workflow

1. Run `cmux`.
2. Triage active sessions first: needs attention, active work, review, stale,
   done/other.
3. Use `tab` to open the full Linear worklist.
4. Start one Linear issue, start a scoping session, or select up to 3 issues for
   a capped batch start.
5. Use `.` on a session for actions such as open, workspace terminal, mark
   scoped, mark needs review, mark done, close session, or reset workspace.

The main dashboard shows only a bounded Linear section. The full Linear
worklist lives behind `tab` or `cmux queue`.

## TUI Keys

```text
↑↓ / j k       navigate
home / end     first / last
pgup / pgdn    move by 5
enter          open session or start Linear issue
.              actions for selected row
a              new work menu
tab            dashboard / Linear worklist
i              show session details
/              filter
space          select Linear issue in full worklist
R              scan sessions and refresh Linear
?              help
q / esc        quit
```

Inside a cmux tmux session, press `prefix + g` to open the popup switcher.
By default tmux prefix is `Ctrl-b`, so press `Ctrl-b` then `g`.

When creating work, cmux asks for the agent once:

```text
↑↓ / j k       choose agent
enter          create
1 / 2 / 3      shortcut to Claude / Codex / Other
```

## Linear Worklist

Set a Linear API key before using Linear-backed features:

```bash
export LINEAR_API_KEY=lin_api_xxx
```

Then run:

```bash
cmux queue setup
```

Queue presets live in `~/.cmux/config.json`. A preset can include teams,
workflow states, labels, assignee mode, priorities, a limit, and a default repo
path. State selection is ordered: the chosen state order controls both which
issues are included and how the worklist is sorted. If no states are configured,
cmux uses:

```text
In Progress -> Todo -> Scoping -> Backlog
```

Starting work from the Linear worklist shows a repo picker. The picker includes
the preset repo, previously used repos, the current directory, and a custom path.
Custom repo paths are remembered after use. Linear work requires a git repo;
scratch sessions can use non-git folders.

If a Linear issue already has a cmux session, cmux opens or recovers that
session instead of starting duplicate work.

## Session Lifecycle

The action menu is opened with `.` on a selected row.

Common actions:

- `Open agent`: attach to the agent session.
- `Workspace terminal`: open an attached shell in the workspace.
- `Mark scoped`: finish a scoping session and move the Linear issue to the ready
  state.
- `Mark needs review`: add the Linear `needs-review` label and put the session
  in the review group.
- `Mark done`: verify the workspace is clean, move the Linear issue to `Done`,
  and place it at the top of the Linear Done column.
- `Close session`: stop the agent, remove a clean cmux-owned worktree, and hide
  the local session. Dirty workspaces are refused.
- `Forget session`: stop and hide the local session while keeping the workspace
  and branch.
- `Reset workspace`: destructive; runs `git reset --hard` and `git clean -fd`
  for a cmux-owned worktree.

`Close session` and `Forget session` show a confirmation pane. `Reset workspace`
requires typing the session id.

## Runbooks And Scoping

Every structured session has a local runbook:

```text
~/.cmux/sessions/<session-id>/RUNBOOK.md
```

The runbook is local durable context for the agent: current state, decisions,
blockers, tests, and next steps. It should contain technical context, not a
mirror of Linear status.

For scoping sessions, the runbook becomes the handoff. When `Mark scoped` or
`cmux agent scoped` runs, cmux reads the useful runbook sections and writes a
replaceable `cmux scoped handoff` block into the Linear issue description. That
lets the next coding agent start from Linear alone, even if the scoping tmux
session is gone.

The Linear scoped handoff preserves markdown and skips local/status-only
placeholders such as `- None.`.

## Runtime Scan

`R` in the TUI or `cmux agent scan` inspects runtime state:

- tmux session/pane existence
- dead panes and exit state
- last activity/current command
- recent terminal output
- git dirty summary

Scan updates conservative runtime statuses such as `crashed`,
`waiting_for_input`, and `stale`. Manual statuses like blocked, review, done,
and PR opened are preserved unless the runtime state makes them impossible.

## Storage

cmux stores its own data in one home directory:

```text
~/.cmux/
  config.json
  sessions/
  worktrees/
  hooks/
```

Override it with:

```bash
CMUX_HOME=/path/to/cmux-home cmux
```

Session metadata lives at:

```text
~/.cmux/sessions/<session-id>/session.json
~/.cmux/sessions/<session-id>/RUNBOOK.md
```

## Configuration

Main config path:

```text
~/.cmux/config.json
```

Important fields:

- `defaultAgent`: default agent command when one is not chosen.
- `repos`: remembered repositories for the repo picker.
- `queuePresets`: saved Linear worklist presets.
- `linear.workflow.transitions`: Linear lifecycle mapping for start, scoped,
  review, done, and abandon actions.

Default Linear workflow behavior:

- Start scoping -> `Scoping`, add `cmux`, remove `needs-review`
- Mark scoped -> `Todo`, add `cmux`, remove `needs-review`
- Start work -> `In Progress`, add `cmux`, remove `needs-review`
- Mark needs review -> add `cmux` and `needs-review`
- Mark done -> `Done`, remove `needs-review`, place at top of Done
- Abandon -> original queue state, remove `needs-review`

Workflow names are configurable so cmux can adapt to different Linear boards.

## CLI Reference

The TUI is the normal workflow, but CLI hooks are useful for scripting and
agent-invoked lifecycle updates.

```text
cmux
cmux new [--agent|-a <cmd>] [--title|-t <text>] [--mobile|-m] [--no-attach] [path]
cmux list
cmux attach <name>
cmux switch
cmux kill <name>
cmux title <text>
cmux info
cmux doctor
cmux debug

cmux agent start [ISSUE] [--scope] [--agent <cmd>] [--title <text>] [--worktree] [--no-worktree] [--prepare]
cmux agent scratch [--agent <cmd>] [--title <text>]
cmux agent list
cmux agent open <id>
cmux agent path <id>
cmux agent scan
cmux agent restart <id>
cmux agent cleanup <id> [--force]
cmux agent reset <id> --confirm <id>
cmux agent scoped <id> [summary]
cmux agent needs-review <id> [summary]
cmux agent abandon <id> [summary]
cmux agent status <id> <status> [summary]
cmux agent block <id> [summary]
cmux agent review <id> [summary]
cmux agent done <id> [summary]

cmux queue
cmux queue list [preset]
cmux queue setup
```

## Development

Run tests:

```bash
go test ./...
```

Check local setup:

```bash
cmux doctor
```
