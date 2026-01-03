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
cmux              # Show interactive session selector (from outside tmux)
cmux switch       # Switch sessions (from inside tmux, mobile-friendly)
```

### Session Management

```bash
cmux new ~/projects/my-app    # Create new session, auto-launches claude
cmux list                     # List all sessions
cmux attach my-app            # Attach to session
cmux kill my-app              # Kill a session
```

### Session Info

```bash
cmux title "fixing auth bug"  # Set session title
cmux rename new-name          # Rename current session
cmux info                     # Show current session info
```

## Selector UI

```
CMUX Sessions

rebar-cosmos
   1) aggregation-logic  [fixing auth bug]     2h
   2) builtins           [adding test suite]   15m

rebar-cosmos-frontend
   3) dashboard          —                     1d

────────────────────────
[n] New session
[q] Quit

Select: _
```

Sessions are grouped by parent directory and show:
- Session name (child directory)
- Title in brackets (if set)
- Session age

## Session Naming

Sessions are named based on directory structure using `@` as separator:

```
/projects/rebar-cosmos/aggregation-logic → cmux@rebar-cosmos@aggregation-logic
```

If a duplicate exists, a numeric suffix is added (e.g., `cmux@rebar-cosmos@aggregation-logic-2`).

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

| Command | Description |
|---------|-------------|
| `cmux` | Interactive session selector |
| `cmux new [path]` | Create new session (default: cwd) |
| `cmux list` | List all sessions |
| `cmux attach <name>` | Attach to session |
| `cmux switch` | Switch sessions (inside tmux) |
| `cmux kill <name>` | Kill a session |
| `cmux rename <name>` | Rename current session |
| `cmux title <text>` | Set session title |
| `cmux info` | Show current session info |
| `cmux help` | Show help |
