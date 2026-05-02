import {basename, dirname, resolve} from "node:path";
import {spawn} from "node:child_process";
import {commandExists, run} from "./process.js";

const prefix = "cmux";
const sep = "@";
const mobileWidth = 78;

export interface TmuxSession {
  name: string;
  created: number;
  title?: string;
  dir?: string;
  agent?: string;
}

export async function ensureTmux(): Promise<void> {
  if (!(await commandExists("tmux"))) {
    throw new Error("tmux is not installed");
  }
}

export function isInsideTmux(): boolean {
  return Boolean(process.env.TMUX);
}

export async function listTmuxSessions(): Promise<TmuxSession[]> {
  try {
    const {stdout} = await run("tmux", ["list-sessions", "-F", "#{session_name}|#{session_created}"]);
    const rows = stdout
      .split("\n")
      .filter(Boolean)
      .filter(line => line.startsWith(`${prefix}${sep}`));
    const sessions: TmuxSession[] = [];
    for (const row of rows) {
      const [name, created] = row.split("|");
      sessions.push({
        name,
        created: Number(created),
        title: await showEnvironment(name, "CMUX_TITLE"),
        dir: await showEnvironment(name, "CMUX_DIR"),
        agent: await showEnvironment(name, "CMUX_AGENT")
      });
    }
    return sessions;
  } catch {
    return [];
  }
}

export async function showEnvironment(session: string, key: string): Promise<string | undefined> {
  try {
    const {stdout} = await run("tmux", ["show-environment", "-t", session, key]);
    const trimmed = stdout.trim();
    if (!trimmed.startsWith(`${key}=`)) return undefined;
    return trimmed.slice(key.length + 1);
  } catch {
    return undefined;
  }
}

export function sessionParent(name: string): string {
  return name.split(sep)[1] ?? "";
}

export function sessionChild(name: string): string {
  return name.split(sep).slice(2).join(sep);
}

export async function generateSessionName(path: string, preferred?: string): Promise<string> {
  const safePreferred = preferred?.replaceAll(sep, "-");
  const absPath = resolve(path);
  const base = safePreferred
    ? `${prefix}${sep}agent${sep}${safePreferred}`
    : `${prefix}${sep}${basename(dirname(absPath))}${sep}${basename(absPath)}`;
  let name = base;
  let counter = 2;
  while (await hasSession(name)) {
    name = `${base}-${counter}`;
    counter += 1;
  }
  return name;
}

export async function hasSession(session: string): Promise<boolean> {
  try {
    await run("tmux", ["has-session", "-t", session]);
    return true;
  } catch {
    return false;
  }
}

export async function createSession(options: {
  name: string;
  cwd: string;
  command?: string;
  title?: string;
  agent?: string;
  mobile?: boolean;
  metadata?: Record<string, string | undefined>;
}): Promise<void> {
  const sizeArgs = options.mobile ? ["-x", String(mobileWidth)] : [];
  await run("tmux", ["new-session", "-d", "-s", options.name, "-c", options.cwd, ...sizeArgs]);
  await setEnvironment(options.name, "CMUX_SESSION", options.name);
  await setEnvironment(options.name, "CMUX_DIR", options.cwd);
  if (options.agent) await setEnvironment(options.name, "CMUX_AGENT", options.agent);
  if (options.title) await setEnvironment(options.name, "CMUX_TITLE", options.title);
  for (const [key, value] of Object.entries(options.metadata ?? {})) {
    if (value !== undefined) await setEnvironment(options.name, key, value);
  }
  await ensurePopupBinding();
  if (options.command) {
    await run("tmux", ["send-keys", "-t", options.name, `exec ${options.command}`, "Enter"]);
  }
}

export async function setEnvironment(session: string, key: string, value: string): Promise<void> {
  await run("tmux", ["set-environment", "-t", session, key, value]);
}

export async function attach(session: string): Promise<void> {
  await ensurePopupBinding();
  if (isInsideTmux()) {
    await run("tmux", ["switch-client", "-t", session]);
  } else {
    await new Promise<void>((resolve, reject) => {
      const child = spawn("tmux", ["attach-session", "-t", session], {stdio: "inherit"});
      child.on("error", reject);
      child.on("exit", () => resolve());
    });
  }
}

export async function kill(session: string): Promise<void> {
  await run("tmux", ["kill-session", "-t", session]);
}

export async function renameSession(current: string, nextChild: string): Promise<string> {
  const next = `${prefix}${sep}${sessionParent(current)}${sep}${nextChild.replaceAll(sep, "-")}`;
  await run("tmux", ["rename-session", "-t", current, next]);
  await setEnvironment(next, "CMUX_SESSION", next);
  return next;
}

export async function getCurrentSession(): Promise<string> {
  const {stdout} = await run("tmux", ["display-message", "-p", "#{session_name}"]);
  return stdout.trim();
}

export async function findSession(target: string): Promise<TmuxSession | undefined> {
  const sessions = await listTmuxSessions();
  return sessions.find(session => session.name.endsWith(`${sep}${target}`))
    ?? sessions.find(session => session.name.includes(target));
}

export async function capturePreview(session: string, lines = 12): Promise<string> {
  try {
    const {stdout} = await run("tmux", ["capture-pane", "-t", session, "-p", "-S", `-${lines * 4}`]);
    return stdout
      .replace(/\x1b\[[0-9;]*[a-zA-Z]/g, "")
      .split("\n")
      .map(line => line.trimEnd())
      .filter(line => line.trim() && !line.includes("for shortcuts") && !line.includes("for help"))
      .slice(-lines)
      .join("\n");
  } catch {
    return "";
  }
}

export async function inferPaneStatus(session: string): Promise<"running" | "waiting_for_input" | "idle" | "crashed"> {
  try {
    const [{stdout: activity}, preview] = await Promise.all([
      run("tmux", ["display-message", "-t", session, "-p", "#{pane_last_activity}"]),
      capturePreview(session, 6)
    ]);
    const idleSeconds = Math.floor(Date.now() / 1000) - Number(activity.trim());
    const lastLine = preview.split("\n").filter(Boolean).at(-1) ?? "";
    if (/error|exception|failed|panic|fatal/i.test(lastLine)) return "crashed";
    if (idleSeconds < 2) return "running";
    if (/(^>|› |\$ ?$|% ?$|❯ ?$)/.test(lastLine)) return "waiting_for_input";
    return "idle";
  } catch {
    return "crashed";
  }
}

export async function pastePrompt(session: string, prompt: string): Promise<void> {
  await run("tmux", ["load-buffer", "-b", "cmux-agent-prompt", "-"], {input: prompt});
  await run("tmux", ["paste-buffer", "-t", session, "-b", "cmux-agent-prompt"]);
  await run("tmux", ["send-keys", "-t", session, "Enter"]);
}

export async function ensurePopupBinding(): Promise<void> {
  try {
    await run("tmux", ["bind-key", "g", "display-popup", "-E", "-w", "85%", "-h", "85%", "cmux legacy switch"]);
  } catch {
    // Binding failure should not block session creation.
  }
}
