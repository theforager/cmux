# cmux

Claude Tmux Session Manager - manage multiple Claude Code sessions via tmux.

Designed for remote access via Terminus (iOS) and other SSH clients.

## Installation

```bash
git clone <repo-url> cmux
cd cmux
./install.sh
```

The installer will:
- Symlink `cmux` to `~/bin`

## Usage

### Session Selector

```bash
cmux              # Show interactive session selector
cmux switch       # Switch sessions (from inside tmux)
```

### Session Management

```bash
cmux new ~/projects/my-app           # Create new session (claude by default)
cmux new -a codex ~/projects/my-app  # Create session running codex instead
cmux new -m ~/projects/my-app        # Create mobile-friendly session (78 cols)
cmux new -t "fixing bugs" ~/my-app   # Create session with title
cmux list                            # List all sessions
cmux attach my-app                   # Attach to session
cmux kill my-app                     # Kill a session
```

### Agents

cmux launches Claude Code by default. Pass `-a codex` (or set `CMUX_AGENT=codex`)
to run [Codex](https://github.com/openai/codex) instead. You can also pass any
custom command (`-a "myagent --flag"`) — cmux just `exec`s whatever you give it.

The selector's `n` (new) flow shows a small picker after the path prompt:

```
  agent
    1 ▸ claude
    2   codex
    3   other (custom command)
```

Use `↑/↓` (or `j/k`, or press `1`/`2`/`3`) and Enter to pick. Picking `other`
prompts for the command to run. The agent binary's first word must be on
`$PATH`; cmux verifies before creating the session so typos fail clearly.

After a session is created, cmux asks you to **press Enter to attach** rather
than throwing you in automatically. Press `q` to leave the session running in
the background; you can attach later from the selector.

`cmux info` shows which agent the current session is running.

### Session Info

```bash
cmux title "fixing auth bug"  # Set session title
cmux rename new-name          # Rename current session
cmux info                     # Show current session info
```

### One-step SSH

```bash
cmux ssh dev@devship          # SSH into host and open selector in one step
cmux ssh dev                  # Same, using an alias from ~/.config/cmux/hosts
```

Define aliases in `~/.config/cmux/hosts`, one per line:

```
dev=dev@devship
prod=-p 2222 user@prod.example.com
```

The value is passed to `ssh -t` so it can include flags like `-p`.

### Popup switcher (jump back from inside a session)

From inside any cmux session, press **prefix + g** to pop the selector as
a floating window. Pick a session, popup closes, you're switched — no
detaching, no exiting Claude.

> The tmux **prefix** is whatever key combo opens tmux commands. By default
> it's `Ctrl-b`, so press `Ctrl-b` then `g`. If you've remapped your prefix
> (e.g. to `Ctrl-a`), use that instead. Check yours with
> `tmux show-options -g prefix`.

**No setup required.** cmux registers the binding on your tmux server
automatically every time you run it. The binding lives until the tmux
server dies (typically only on reboot or `tmux kill-server`), and the
next `cmux` invocation re-registers it. We deliberately use `prefix + g`
rather than `prefix + s` so we don't clobber tmux's built-in session
picker — `g` has no default mapping in tmux.

If you want the binding to survive `tmux kill-server` without running
cmux first, paste this one line into `~/.tmux.conf` yourself:

```
bind-key g display-popup -E -w 85% -h 85% "cmux switch"
```

Requires tmux 3.2+ for `display-popup`.

## Session Lifecycle

cmux has three layers — a selector, tmux sessions, and Claude running inside
each session. Knowing which "close" maps to which layer keeps things tidy:

| You want to… | Do this | What happens |
|---|---|---|
| **End this session for good** | Quit Claude (`Ctrl-D` or `/exit`) | Claude exits; the tmux session auto-ends and disappears from the selector |
| **Step away, keep working later** | `prefix d` (default `Ctrl-b d`) | Detaches the tmux client; session keeps running in the background |
| **Close the selector without picking** | `q` or `esc` | Just closes the selector view; nothing else changes |
| **Force-kill another session** | `d` on it in the selector (or `cmux kill <name>`) | Confirms, then destroys that session |

Because cmux launches Claude with `exec`, ending Claude ends the session —
no orphaned shell prompts to clean up.

## Status Indicators

The selector shows real-time session status:

| Symbol | Status | Description |
|--------|--------|-------------|
| ● | Running | Claude is actively generating output |
| ◐ | Waiting | Claude is waiting for your input |
| ○ | Idle | Session has been idle for a while |
| ✕ | Error | Error detected in session |

## Selector UI

```
CMUX  ● running  ◐ waiting  ○ idle  ✕ error
my-project
◐ ▸ 1  fixing auth bug (api)  2h
○   2  worker  15m
──────────────────────────────────────────────────────────────
> Can you help me fix the auth bug?
I'll help you fix the authentication bug. Let me start by
looking at the auth middleware to understand the current flow.
──────────────────────────────────────────────────────────────
↑↓ navigate · enter select · [n] new · [d] delete · [q] quit
```

Features:
- **Status indicators** - See which sessions are active at a glance
- **Session preview** - View recent conversation content for selected session
- **Adaptive layout** - Preview height adjusts to terminal size (2-8 lines)
- **Title as primary name** - Custom titles display prominently with folder in parentheses
- **Keyboard navigation** - Arrow keys, vim keys (j/k), home/end, pgup/pgdn
- **Multi-digit jump** - Type `12` to jump to session #12
- **`/` filter** - Live filter sessions by parent / child / title
- **In-selector rename / title / delete** - `r`, `t`, `d` act on the selected row
- **Help overlay** - `?` shows the full key map

## Options

| Flag | Description |
|------|-------------|
| `-m, --mobile` | Create session at fixed 78-col width (for mobile clients) |
| `-t, --title` | Set session title on creation |

## Session Naming

Sessions are named based on directory structure using `@` as separator:

```
/projects/my-project/api → cmux@my-project@api
```

If a duplicate exists, a numeric suffix is added (e.g., `cmux@my-project@api-2`).

## Terminus Setup

To show the session selector on connect via Terminus:

1. Open Terminus settings for your host
2. Set "Startup Command" to: `cmux`

## Commands Reference

| Command | Alias | Description |
|---------|-------|-------------|
| `cmux` | | Interactive session selector |
| `cmux selector` | `s` | Interactive session selector |
| `cmux new [opts] [path]` | | Create new session (default: cwd) |
| `cmux list` | `ls` | List all sessions with status |
| `cmux attach <name>` | `a` | Attach to session |
| `cmux switch` | `sw` | Switch sessions (inside tmux) |
| `cmux kill <name>` | `k` | Kill a session |
| `cmux rename <name>` | | Rename current session |
| `cmux title <text>` | `t` | Set session title |
| `cmux info` | `i` | Show current session info |
| `cmux ssh <host\|alias>` | | One-step SSH + selector |
| `cmux help` | | Show help |
