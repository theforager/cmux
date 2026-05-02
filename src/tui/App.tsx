import React, {useEffect, useMemo, useState} from "react";
import {Box, Text, useApp, useInput} from "ink";
import {basename, resolve} from "node:path";
import {killAgent, startAgent, updateAgentStatus} from "../core/agents.js";
import {age, pad} from "../core/format.js";
import {listSessions, updateSession} from "../core/state.js";
import {capturePreview, inferPaneStatus, kill, listTmuxSessions, renameSession, sessionChild, sessionParent, setEnvironment} from "../core/tmux.js";
import type {AgentSession} from "../core/types.js";

interface Row {
  id: string;
  title: string;
  subtitle: string;
  status: string;
  updated: string;
  tmuxSession: string;
  preview: string;
  active: boolean;
  registered?: AgentSession;
}

type UiMode =
  | "browse"
  | "filter"
  | "help"
  | "rename"
  | "title"
  | "delete"
  | "newScratch"
  | "agentStart"
  | "block"
  | "review"
  | "fail";

export function App({mode: _mode, onOpen}: {mode: "attach" | "switch"; onOpen: (session: string) => void}): React.ReactElement {
  const {exit} = useApp();
  const [rows, setRows] = useState<Row[]>([]);
  const [selected, setSelected] = useState(0);
  const [filter, setFilter] = useState("");
  const [uiMode, setUiMode] = useState<UiMode>("browse");
  const [text, setText] = useState("");
  const [digits, setDigits] = useState("");
  const [message, setMessage] = useState("");

  async function refresh(): Promise<void> {
    const registered = listSessions();
    const tmux = await listTmuxSessions();
    const next: Row[] = [];
    const byTmux = new Map(registered.map(session => [session.tmuxSession, session]));
    for (const tmuxSession of tmux) {
      const session = byTmux.get(tmuxSession.name);
      const inferred = session ? session.status : await inferPaneStatus(tmuxSession.name);
      next.push({
        id: session?.id ?? sessionChild(tmuxSession.name),
        title: session?.title ?? tmuxSession.title ?? sessionChild(tmuxSession.name),
        subtitle: session
          ? `${session.type}${session.branch ? ` · ${session.branch}` : ""}`
          : `${sessionParent(tmuxSession.name)} / ${sessionChild(tmuxSession.name)}`,
        status: inferred,
        updated: session ? age(session.lastUpdatedAt) : age(tmuxSession.created),
        tmuxSession: tmuxSession.name,
        preview: await capturePreview(tmuxSession.name, 8),
        active: true,
        registered: session
      });
    }
    for (const session of registered) {
      if (tmux.some(item => item.name === session.tmuxSession)) continue;
      next.push({
        id: session.id,
        title: session.title,
        subtitle: `${session.type}${session.branch ? ` · ${session.branch}` : ""}`,
        status: session.status === "running" ? "crashed" : session.status,
        updated: age(session.lastUpdatedAt),
        tmuxSession: session.tmuxSession,
        preview: session.lastSummary ?? "",
        active: false,
        registered: session
      });
    }
    setRows(next);
    setSelected(value => Math.min(value, Math.max(0, next.length - 1)));
  }

  useEffect(() => {
    void refresh();
    const timer = setInterval(() => void refresh(), 3000);
    return () => {
      clearInterval(timer);
    };
  }, []);

  const visible = useMemo(() => {
    const needle = filter.toLowerCase();
    if (!needle) return rows;
    return rows.filter(row => `${row.id} ${row.title} ${row.subtitle} ${row.status}`.toLowerCase().includes(needle));
  }, [rows, filter]);

  useInput((input, key) => {
    setMessage("");
    const row = visible[selected];

    if (uiMode === "help") {
      setUiMode("browse");
      return;
    }

    if (uiMode !== "browse") {
      setDigits("");
      if (key.escape) {
        setUiMode("browse");
        setText("");
        return;
      }
      if (key.backspace || key.delete) {
        setText(value => value.slice(0, -1));
        if (uiMode === "filter") setFilter(value => value.slice(0, -1));
        return;
      }
      if (key.return) {
        void commitMode(uiMode, text, row).then(() => {
          setUiMode("browse");
          setText("");
          void refresh();
        }).catch(error => {
          setMessage(error instanceof Error ? error.message : String(error));
        });
        return;
      }
      if (input && input.length === 1) {
        setText(value => value + input);
        if (uiMode === "filter") {
          setFilter(value => value + input);
          setSelected(0);
        }
      }
      return;
    }

    if (input === "q" || key.escape) {
      exit();
      return;
    }
    if (/^[0-9]$/.test(input) && input !== "0") {
      const nextDigits = `${digits}${input}`;
      const target = Number(nextDigits);
      if (target >= 1 && target <= visible.length) {
        setDigits(nextDigits);
        setSelected(target - 1);
      } else {
        const single = Number(input);
        setDigits(single >= 1 && single <= visible.length ? input : "");
        if (single >= 1 && single <= visible.length) setSelected(single - 1);
      }
      return;
    }
    setDigits("");
    if (input === "?") {
      setUiMode("help");
      return;
    }
    if (input === "/") {
      setUiMode("filter");
      setText(filter);
      return;
    }
    if (input === "n") {
      setUiMode("newScratch");
      setText(process.cwd());
      return;
    }
    if (input === "a") {
      setUiMode("agentStart");
      setText("");
      return;
    }
    if (input === "r" && row) {
      setUiMode("rename");
      setText(sessionChild(row.tmuxSession));
      return;
    }
    if (input === "t" && row) {
      setUiMode("title");
      setText(row.title);
      return;
    }
    if (input === "d" && row) {
      setUiMode("delete");
      setText("");
      return;
    }
    if (input === "b" && row?.registered) {
      setUiMode("block");
      setText("");
      return;
    }
    if (input === "f" && row?.registered) {
      setUiMode("fail");
      setText("");
      return;
    }
    if (input === "v" && row?.registered) {
      setUiMode("review");
      setText("");
      return;
    }
    if (input === "x" && row?.registered) {
      void updateAgentStatus(row.registered.id, "done", "Marked done from cmux TUI")
        .then(() => refresh())
        .catch(error => setMessage(error instanceof Error ? error.message : String(error)));
      return;
    }
    if (input === "R") {
      void refresh();
      return;
    }
    if (key.upArrow || input === "k") {
      setSelected(value => Math.max(0, value - 1));
      return;
    }
    if (key.downArrow || input === "j") {
      setSelected(value => Math.min(Math.max(0, visible.length - 1), value + 1));
      return;
    }
    if (key.pageUp) {
      setSelected(value => Math.max(0, value - 5));
      return;
    }
    if (key.pageDown) {
      setSelected(value => Math.min(Math.max(0, visible.length - 1), value + 5));
      return;
    }
    if (key.leftArrow || key.home) {
      setSelected(0);
      return;
    }
    if (key.rightArrow || key.end) {
      setSelected(Math.max(0, visible.length - 1));
      return;
    }
    if (key.return) {
      if (!row) return;
      if (!row.active) {
        setMessage(`Session is not running: ${row.id}`);
        return;
      }
      onOpen(row.tmuxSession);
      exit();
    }
  });

  const row = visible[selected];
  if (uiMode === "help") {
    return <Help />;
  }

  return (
    <Box flexDirection="column">
      <Box>
        <Text bold color="cyan">cmux</Text>
        <Text dimColor>  {visible.length} session{visible.length === 1 ? "" : "s"}  </Text>
        {filter || uiMode === "filter" ? <Text color="yellow">/{filter}{uiMode === "filter" ? "_" : ""}</Text> : null}
      </Box>
      <Box flexDirection="column" marginTop={1}>
        {visible.length === 0 ? (
          <Text dimColor>  no sessions</Text>
        ) : renderRows(visible, selected)}
      </Box>
      <Box marginTop={1} flexDirection="column">
        <Text dimColor>── preview ─────────────────────────────────────────</Text>
        {row?.preview
          ? row.preview.split("\n").slice(-8).map((line, index) => <Text key={index}>▎ {line}</Text>)
          : <Text dimColor>▎ no recent activity</Text>}
      </Box>
      <Box marginTop={1}>
        {renderPrompt(uiMode, text, row)}
        {uiMode === "browse" ? <Text dimColor>↑↓/jk nav  1-9 jump  home/end  pgup/pgdn  enter open  / filter  n scratch  a issue/task  r rename  t title  d delete  b/f/v/x status  ? help  q quit</Text> : null}
        {digits ? <Text color="cyan">  → {digits}</Text> : null}
        {message ? <Text color="red">  {message}</Text> : null}
      </Box>
    </Box>
  );

  async function commitMode(currentMode: UiMode, value: string, row?: Row): Promise<void> {
    const trimmed = value.trim();
    switch (currentMode) {
      case "filter":
        return;
      case "newScratch": {
        const cwd = resolve(trimmed || process.cwd());
        await startAgent({cwd, scratch: true, title: basename(cwd) || "Scratch session"});
        return;
      }
      case "agentStart": {
        if (!trimmed) throw new Error("Enter a Linear issue key or task title");
        if (/^[A-Z][A-Z0-9]+-\d+$/.test(trimmed)) {
          await startAgent({cwd: process.cwd(), issueKey: trimmed});
        } else {
          await startAgent({cwd: process.cwd(), title: trimmed, worktree: true});
        }
        return;
      }
      case "rename": {
        if (!row || !trimmed) throw new Error("No session selected");
        const next = await renameSession(row.tmuxSession, trimmed);
        if (row.registered) {
          updateSession(row.registered.id, session => ({...session, tmuxSession: next}));
        }
        return;
      }
      case "title": {
        if (!row) throw new Error("No session selected");
        await setEnvironment(row.tmuxSession, "CMUX_TITLE", trimmed);
        if (row.registered) {
          updateSession(row.registered.id, session => ({...session, title: trimmed || session.title}));
        }
        return;
      }
      case "delete": {
        if (!row) throw new Error("No session selected");
        if (trimmed.toLowerCase() !== "y") return;
        if (row.registered) {
          await killAgent(row.registered.id);
        } else {
          await kill(row.tmuxSession);
        }
        return;
      }
      case "block": {
        if (!row?.registered) throw new Error("Selected row is not a structured agent session");
        await updateAgentStatus(row.registered.id, "blocked", trimmed || "Blocked from cmux TUI");
        return;
      }
      case "fail": {
        if (!row?.registered) throw new Error("Selected row is not a structured agent session");
        await updateAgentStatus(row.registered.id, "tests_failed", trimmed || "Tests failed");
        return;
      }
      case "review": {
        if (!row?.registered) throw new Error("Selected row is not a structured agent session");
        await updateAgentStatus(row.registered.id, "ready_for_review", trimmed || "Ready for review");
        return;
      }
      default:
        return;
    }
  }
}

function renderRows(rows: Row[], selected: number): React.ReactElement[] {
  const elements: React.ReactElement[] = [];
  let previousGroup = "";
  for (const [index, item] of rows.entries()) {
    const group = item.registered?.type ?? sessionParent(item.tmuxSession) ?? "sessions";
    if (group !== previousGroup) {
      previousGroup = group;
      elements.push(<Text key={`group:${group}:${index}`} dimColor>  {group}</Text>);
    }
    elements.push(
      <Text key={`${item.tmuxSession}:${item.id}:${index}`} color={index === selected ? "green" : undefined}>
        {index === selected ? "▌" : " "}
        {" "}
        {statusGlyph(item.status)}
        {" "}
        {pad(String(index + 1), 3)}
        {" "}
        {pad(item.id, 16)}
        {" "}
        {pad(item.status, 17)}
        {" "}
        {item.active ? item.title : `${item.title} (not running)`}
        <Text dimColor>  {item.updated}</Text>
      </Text>
    );
  }
  return elements;
}

function renderPrompt(mode: UiMode, text: string, row?: Row): React.ReactElement | null {
  switch (mode) {
    case "filter":
      return <Text color="yellow">filter  {text}_  <Text dimColor>enter apply · esc cancel</Text></Text>;
    case "newScratch":
      return <Text color="yellow">scratch path  {text}_  <Text dimColor>enter create · esc cancel</Text></Text>;
    case "agentStart":
      return <Text color="yellow">issue/task  {text}_  <Text dimColor>REB-123 starts Linear work · other text creates task worktree</Text></Text>;
    case "rename":
      return <Text color="yellow">rename  {text}_  <Text dimColor>enter save · esc cancel</Text></Text>;
    case "title":
      return <Text color="yellow">title  {text}_  <Text dimColor>enter save · esc cancel</Text></Text>;
    case "delete":
      return <Text color="red">delete {row?.id ?? ""}? type y then enter  <Text dimColor>esc cancel</Text></Text>;
    case "block":
      return <Text color="yellow">block reason  {text}_  <Text dimColor>syncs status/hooks/Linear</Text></Text>;
    case "fail":
      return <Text color="red">failure summary  {text}_  <Text dimColor>syncs status/hooks/Linear</Text></Text>;
    case "review":
      return <Text color="green">review summary  {text}_  <Text dimColor>syncs status/hooks/Linear</Text></Text>;
    default:
      return null;
  }
}

function Help(): React.ReactElement {
  return (
    <Box flexDirection="column">
      <Text bold color="cyan">CMUX Keys</Text>
      <Text> </Text>
      <Text><Text color="cyan">↑↓ j k</Text>        navigate</Text>
      <Text><Text color="cyan">home end</Text>      first / last</Text>
      <Text><Text color="cyan">pgup pgdn</Text>     move by 5</Text>
      <Text><Text color="cyan">1-9 digits</Text>    jump to row number</Text>
      <Text><Text color="cyan">enter</Text>         open selected session</Text>
      <Text><Text color="cyan">/</Text>             filter rows</Text>
      <Text><Text color="cyan">n</Text>             create scratch session at a path</Text>
      <Text><Text color="cyan">a</Text>             start Linear issue or task-backed agent</Text>
      <Text><Text color="cyan">r</Text>             rename selected tmux session</Text>
      <Text><Text color="cyan">t</Text>             set selected title</Text>
      <Text><Text color="cyan">d</Text>             delete selected session</Text>
      <Text><Text color="cyan">b</Text>             mark structured agent blocked</Text>
      <Text><Text color="cyan">f</Text>             mark structured agent tests_failed</Text>
      <Text><Text color="cyan">v</Text>             mark structured agent ready_for_review</Text>
      <Text><Text color="cyan">x</Text>             mark structured agent done</Text>
      <Text><Text color="cyan">R</Text>             refresh immediately</Text>
      <Text><Text color="cyan">q esc</Text>         quit</Text>
      <Text> </Text>
      <Text dimColor>Agent start: enter REB-123 for Linear issue-backed work, or free text for a task-backed worktree.</Text>
      <Text dimColor>Status changes run hooks and sync the managed Linear comment when configured.</Text>
      <Text> </Text>
      <Text dimColor>press any key to return</Text>
    </Box>
  );
}

function statusGlyph(status: string): string {
  switch (status) {
    case "running": return "●";
    case "waiting_for_input": return "◐";
    case "blocked": return "!";
    case "tests_failed": return "✕";
    case "ready_for_review": return "◆";
    case "done": return "✓";
    case "crashed": return "✕";
    default: return "○";
  }
}
