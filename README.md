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
- Optionally install the `/cmux_name` slash command for Claude Code

## Usage

### Session Selector

```bash
cmux              # Show interactive session selector
cmux switch       # Switch sessions (from inside tmux)
```

### Session Management

```bash
cmux new ~/projects/my-app           # Create new session
cmux new -m ~/projects/my-app        # Create mobile-friendly session (78 cols)
cmux new -t "fixing bugs" ~/my-app   # Create session with title
cmux list                            # List all sessions
cmux attach my-app                   # Attach to session
cmux kill my-app                     # Kill a session
```

### Session Info

```bash
cmux title "fixing auth bug"  # Set session title
cmux rename new-name          # Rename current session
cmux info                     # Show current session info
```

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
- **Keyboard navigation** - Arrow keys, vim keys (j/k), or number keys

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

## Claude Auto-Title

The installer can add a `/cmux_name` slash command to your Claude Code setup.

When you run `/cmux_name`, Claude will analyze the current conversation and set an appropriate session title using `cmux title`.

To manually add it later:

```bash
cp commands/cmux_name.md ~/.claude/commands/
```

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
| `cmux help` | | Show help |
