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

For remote machines, `cmux ssh` opens the remote cmux console in one step:

```bash
cmux ssh dev@devship
cmux ssh dev queue
```

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
2. Triage agent sessions first, then Linear worklist rows, then done / other.
3. Use `tab` to open the full Linear worklist.
4. Start one Linear issue, start a scoping session, or select up to 3 issues for
   a capped batch start.
5. Use `.` on a row for open, submit, abandon, inspect, rename, and advanced
   worktree cleanup actions.

The main dashboard shows only a bounded Linear section. The full Linear
worklist lives behind `tab` or `cmux queue`.

Rows keep local execution state next to local badges, then show Linear state:

```text
REB-135  ◐ waiting ⧉    In Progress   Fix account menu keyboard nav
REB-27   -              Todo          Add pricing CTA tracking
```

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

## Remote Access

`cmux ssh` piggybacks on your normal SSH access and runs cmux on the remote
host:

```bash
cmux ssh user@remote
cmux ssh user@remote queue
cmux ssh user@remote agent list
```

It runs `ssh -t <host> cmux [args...]`, so the remote machine must already have
cmux installed and available on `PATH`.

Host aliases can be stored in:

```text
~/.config/cmux/hosts
```

Format:

```text
dev=dev@devship
prod=-p 2222 user@prod.example.com
```

Then:

```bash
cmux ssh dev
cmux ssh prod queue
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

- `Agent...`: open or restart an existing session, or start a new Linear agent.
  Use left/right or h/l to switch Fresh, Scoping, and Implementation modes. The
  initially selected mode is derived from the Linear state. Fresh starts an
  empty tracked agent without pasting the cmux runbook prompt.
- `Open workspace...`: switch between Terminal, Editor, and Remote modes with
  left/right or h/l. Terminal opens tmux shells, Editor runs local editor
  commands, and Remote shows copyable `cursor --remote`, `code --remote`, or SSH
  cd commands. If no SSH target is saved, cmux prompts once and remembers it.
- `Submit work`: publish the right Linear handoff for the current mode, then
  close the local cmux session. Dirty workspaces are refused so changes are not
  lost.
- `Abandon`: move this attempt back to its queue state and close the local cmux
  session. cmux-owned worktrees are discarded.
- `Rename session`: change the cmux display name.
- `Delete worktree`: advanced escape hatch for removing a cmux-owned worktree.
  Clean worktrees require `y`; dirty worktrees require typing the session id.

## Runbooks And Scoping

Every structured session has a local runbook:

```text
~/.cmux/sessions/<session-id>/RUNBOOK.md
```

The runbook is local durable context for the agent. Implementation sessions keep
it lean: current state, decisions, tests, next action, and review notes. It
should contain technical context, not a mirror of Linear status.

For scoping sessions, the runbook becomes the handoff. When `Submit work` or
`cmux agent scoped` runs, cmux reads the useful runbook sections and writes a
replaceable `cmux scoped handoff` block into the Linear issue description. That
lets the next coding agent start from Linear alone, even if the scoping tmux
session is gone.

Scoping runbooks use a handoff-focused shape:

```text
Goal
Current understanding
Key decisions
Proposed plan
Acceptance criteria
Open questions / risks
User confirmation
Next coding steps
```

Before a scoping session can be marked scoped, the runbook must record user
confirmation. The agent should walk through key decisions, open questions, and
the proposed plan with the user, then record the approval under
`## User confirmation`. This is separate from merely approving the command to
run.

The Linear scoped handoff preserves markdown and skips local/status-only
placeholders such as `- None.`.

## Runtime Scan

`R` in the TUI or `cmux agent scan` inspects runtime state:

- tmux session/pane existence
- dead panes and exit state
- last activity/current command
- recent terminal output
- git dirty summary

Scan updates conservative runtime statuses such as `crashed` and
`waiting_for_input`. `waiting_for_input` is only for approval/permission-style
prompts, not a normal shell or agent prompt. Inactivity is shown as an age, not
promoted into a status.
Manual statuses like blocked, review, done, and PR opened are preserved unless
the runtime state makes them impossible.

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
- `defaultEditorCommand`: first editor command shown in the editor picker.
- `editorCommands`: remembered editor commands and executable paths.
- `defaultSshTarget`: remembered SSH target for remote workspace commands.
- `queuePresets`: saved Linear worklist presets.
- `linear.workflow.transitions`: Linear lifecycle mapping for start, scoped,
  review, done, and abandon actions.

Default Linear workflow behavior:

- Start scoping -> `Scoping`, add `cmux`, remove `needs-review`
- Submit scoping -> `Todo`, add `cmux`, remove `needs-review`
- Start work -> `In Progress`, add `cmux`, remove `needs-review`
- Submit implementation -> add `cmux` and `needs-review`
- Submit review -> `Done`, remove `needs-review`, place at top of Done
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
cmux ssh <host|alias> [cmux-args...]
cmux kill <name>
cmux title <text>
cmux info
cmux doctor
cmux debug

cmux agent start [ISSUE] [--scope] [--fresh] [--agent <cmd>] [--title <text>] [--worktree] [--no-worktree] [--prepare]
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
