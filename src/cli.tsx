#!/usr/bin/env node
import {spawn} from "node:child_process";
import {existsSync, readFileSync} from "node:fs";
import {homedir} from "node:os";
import {dirname, resolve} from "node:path";
import {fileURLToPath} from "node:url";
import {render} from "ink";
import React from "react";
import {startAgent, openAgent, updateAgentStatus, promoteScratch, killAgent, validateAgentCommand} from "./core/agents.js";
import {CmuxError} from "./core/errors.js";
import {age, pad} from "./core/format.js";
import {syncSessionToLinear} from "./core/linear.js";
import {listSessions, readSession, updateSession, writeSession} from "./core/state.js";
import {
  attach,
  createSession,
  ensureTmux,
  findSession,
  getCurrentSession,
  inferPaneStatus,
  isInsideTmux,
  kill,
  listTmuxSessions,
  renameSession,
  sessionChild,
  setEnvironment
} from "./core/tmux.js";
import {runDaemon} from "./daemon/daemon.js";
import {App} from "./tui/App.js";
import {enterFullScreen, leaveFullScreen} from "./core/terminal.js";
import type {AgentStatus} from "./core/types.js";

const version = "2.0.0";

async function main(argv: string[]): Promise<void> {
  const [cmd = "", ...args] = argv;
  if (cmd === "help" || cmd === "--help" || cmd === "-h") {
    help();
    return;
  }
  if (cmd === "version" || cmd === "--version" || cmd === "-v") {
    console.log(`cmux v${version}`);
    return;
  }
  if (cmd === "legacy") {
    await execLegacy(args);
    return;
  }

  await ensureTmux();

  switch (cmd) {
    case "":
      if (!canRunInteractive()) {
        throw new CmuxError("Interactive selector requires a TTY. Run 'cmux help' for commands.");
      }
      await ensureUserStateAvailable();
      await runSelector("attach");
      return;
    case "selector":
    case "s":
      if (!canRunInteractive()) {
        throw new CmuxError("Interactive selector requires a TTY. Run 'cmux help' for commands.");
      }
      await ensureUserStateAvailable();
      await runSelector("attach");
      return;
    case "switch":
    case "sw":
      if (!isInsideTmux()) throw new CmuxError("Not inside tmux. Use 'cmux' instead.");
      await execLegacy(["switch"]);
      return;
    case "new":
      await cmdNew(args);
      return;
    case "list":
    case "ls":
      await cmdList();
      return;
    case "attach":
    case "a":
      await cmdAttach(args);
      return;
    case "kill":
    case "k":
      await cmdKill(args);
      return;
    case "rename":
      await cmdRename(args);
      return;
    case "title":
    case "t":
      await cmdTitle(args);
      return;
    case "info":
    case "i":
      await cmdInfo();
      return;
    case "agent":
      await cmdAgent(args);
      return;
    case "daemon":
      await cmdDaemon(args);
      return;
    case "ssh":
      await cmdSsh(args);
      return;
    case "doctor":
      await cmdDoctor();
      return;
    case "debug":
      await cmdDebug();
      return;
    default:
      throw new CmuxError(`Unknown command: ${cmd}. Run 'cmux help' for usage.`);
  }
}

function canRunInteractive(): boolean {
  return Boolean(process.stdin.isTTY && process.stdout.isTTY && typeof process.stdin.setRawMode === "function");
}

async function runSelector(mode: "attach" | "switch"): Promise<void> {
  let selectedSession: string | undefined;
  enterFullScreen();
  const app = render(<App mode={mode} onOpen={session => {
    selectedSession = session;
  }} />);
  try {
    await app.waitUntilExit();
  } finally {
    leaveFullScreen();
  }
  if (selectedSession) {
    await attach(selectedSession);
  }
}

async function ensureUserStateAvailable(): Promise<void> {
  const {ensureStateDirs} = await import("./core/config.js");
  ensureStateDirs();
}

async function cmdNew(args: string[]): Promise<void> {
  const parsed = parseOptions(args);
  const path = resolve(parsed.positionals[0] ?? ".");
  const agent = parsed.options.agent ?? parsed.options.a ?? process.env.CMUX_AGENT ?? "claude";
  await validateAgentCommand(agent);
  const sessionName = await import("./core/tmux.js").then(mod => mod.generateSessionName(path));
  await createSession({
    name: sessionName,
    cwd: path,
    command: agent,
    title: parsed.options.title ?? parsed.options.t,
    agent: agent.split(/\s+/)[0]?.toLowerCase(),
    mobile: parsed.flags.has("mobile") || parsed.flags.has("m")
  });
  console.log(`Created session: ${sessionName}`);
  if (!parsed.flags.has("no-attach")) await attach(sessionName);
}

async function cmdList(): Promise<void> {
  const tmux = await listTmuxSessions();
  if (tmux.length === 0) {
    console.log("No cmux sessions");
    return;
  }
  for (const session of tmux) {
    const status = await inferPaneStatus(session.name);
    const title = session.title ?? sessionChild(session.name);
    console.log(`${pad(status, 18)} ${pad(title, 32)} ${age(session.created)}  ${session.name}`);
  }
}

async function cmdAttach(args: string[]): Promise<void> {
  const target = args[0];
  if (!target) throw new CmuxError("Usage: cmux attach <name>");
  const session = await findSession(target);
  if (!session) throw new CmuxError(`No session matching: ${target}`);
  await attach(session.name);
}

async function cmdKill(args: string[]): Promise<void> {
  const target = args[0];
  if (!target) throw new CmuxError("Usage: cmux kill <name>");
  const session = await findSession(target);
  if (!session) throw new CmuxError(`No session matching: ${target}`);
  await kill(session.name);
  console.log(`Killed session: ${session.name}`);
}

async function cmdRename(args: string[]): Promise<void> {
  const name = args[0];
  if (!name) throw new CmuxError("Usage: cmux rename <name>");
  const current = await getCurrentSession();
  const next = await renameSession(current, name);
  console.log(`Renamed to: ${next}`);
}

async function cmdTitle(args: string[]): Promise<void> {
  const title = args.join(" ");
  if (!title) throw new CmuxError("Usage: cmux title <text>");
  const current = await getCurrentSession();
  await setEnvironment(current, "CMUX_TITLE", title);
  console.log(`Title set: ${title}`);
}

async function cmdInfo(): Promise<void> {
  const current = await getCurrentSession();
  const sessions = await listTmuxSessions();
  const session = sessions.find(item => item.name === current);
  console.log(`Session: ${current}`);
  console.log(`Directory: ${session?.dir ?? "<unknown>"}`);
  console.log(`Title: ${session?.title ?? "<not set>"}`);
  console.log(`Agent: ${session?.agent ?? "claude"}`);
}

async function cmdAgent(args: string[]): Promise<void> {
  const [sub = "", ...rest] = args;
  switch (sub) {
    case "start": {
      const parsed = parseOptions(rest);
      const issueKey = parsed.positionals[0];
      const session = await startAgent({
        cwd: process.cwd(),
        issueKey,
        title: parsed.options.title ?? parsed.options.t,
        worktree: parsed.flags.has("worktree"),
        noWorktree: parsed.flags.has("no-worktree"),
        prepareOnly: parsed.flags.has("prepare"),
        agent: parsed.options.agent ?? parsed.options.a,
      });
      printAgent(session);
      return;
    }
    case "scratch": {
      const parsed = parseOptions(rest);
      const session = await startAgent({
        cwd: process.cwd(),
        scratch: true,
        title: parsed.options.title ?? parsed.options.t,
        agent: parsed.options.agent ?? parsed.options.a
      });
      printAgent(session);
      return;
    }
    case "list":
    case "ls":
      await agentList();
      return;
    case "open":
    case "attach":
      if (!rest[0]) throw new CmuxError("Usage: cmux agent open <id>");
      await openAgent(rest[0]);
      return;
    case "status": {
      const [id, status, ...summary] = rest;
      if (!id || !status) throw new CmuxError("Usage: cmux agent status <id> <status> [summary]");
      const session = await updateAgentStatus(id, status as AgentStatus, summary.join(" ") || undefined);
      printAgent(session);
      return;
    }
    case "block": {
      const [id, ...summary] = rest;
      if (!id) throw new CmuxError("Usage: cmux agent block <id> <reason>");
      const session = await updateAgentStatus(id, "blocked", summary.join(" ") || undefined);
      printAgent(session);
      return;
    }
    case "review": {
      const [id, ...summary] = rest;
      if (!id) throw new CmuxError("Usage: cmux agent review <id> [summary]");
      const session = await updateAgentStatus(id, "ready_for_review", summary.join(" ") || undefined);
      printAgent(session);
      return;
    }
    case "done": {
      const [id, ...summary] = rest;
      if (!id) throw new CmuxError("Usage: cmux agent done <id> [summary]");
      const session = await updateAgentStatus(id, "done", summary.join(" ") || undefined);
      printAgent(session);
      return;
    }
    case "promote": {
      const parsed = parseOptions(rest);
      const id = parsed.positionals[0];
      const issue = parsed.options.issue;
      if (!id || !issue) throw new CmuxError("Usage: cmux agent promote <scratch-id> --issue <KEY-123>");
      const session = await promoteScratch(id, issue);
      printAgent(session);
      return;
    }
    case "sync": {
      const id = rest[0];
      if (!id) throw new CmuxError("Usage: cmux agent sync <id>");
      const session = readSession(id);
      if (!session) throw new CmuxError(`No agent session found: ${id}`);
      const synced = await syncSessionToLinear(session);
      writeSession(synced);
      printAgent(synced);
      return;
    }
    case "kill": {
      if (!rest[0]) throw new CmuxError("Usage: cmux agent kill <id>");
      await killAgent(rest[0]);
      return;
    }
    default:
      agentHelp();
  }
}

async function agentList(): Promise<void> {
  const sessions = listSessions();
  if (sessions.length === 0) {
    console.log("No cmux agent sessions");
    return;
  }
  console.log(`${pad("ID", 16)} ${pad("STATUS", 17)} ${pad("TYPE", 13)} ${pad("BRANCH", 32)} UPDATED  TITLE`);
  for (const session of sessions) {
    console.log(`${pad(session.id, 16)} ${pad(session.status, 17)} ${pad(session.type, 13)} ${pad(session.branch ?? "current", 32)} ${pad(age(session.lastUpdatedAt), 7)} ${session.title}`);
  }
}

async function cmdDaemon(args: string[]): Promise<void> {
  const parsed = parseOptions(args);
  await runDaemon({cwd: process.cwd(), once: parsed.flags.has("once")});
  if (!parsed.flags.has("once")) {
    console.log("cmux daemon running");
  }
}

async function cmdSsh(args: string[]): Promise<void> {
  const [target, ...cmuxArgs] = args;
  if (!target) throw new CmuxError("Usage: cmux ssh <host|alias> [cmux-args...]");
  const host = resolveSshHost(target).split(/\s+/).filter(Boolean);
  await new Promise<void>((resolvePromise, reject) => {
    const child = spawn("ssh", ["-t", ...host, "cmux", ...cmuxArgs], {stdio: "inherit"});
    child.on("error", reject);
    child.on("exit", code => code === 0 ? resolvePromise() : reject(new Error(`ssh exited with ${code}`)));
  });
}

async function cmdDoctor(): Promise<void> {
  const {existsSync} = await import("node:fs");
  const {getStateDir, configPath, loadConfig} = await import("./core/config.js");
  const config = loadConfig();
  console.log(`cmux: ${version}`);
  console.log(`node: ${process.version}`);
  console.log(`entrypoint: ${fileURLToPath(import.meta.url)}`);
  console.log(`built: ${existsSync(fileURLToPath(import.meta.url)) ? "yes" : "no"}`);
  console.log(`tmux: ${await commandAvailable("tmux") ? "yes" : "no"}`);
  console.log(`config: ${configPath()}`);
  console.log(`state: ${getStateDir(config)}`);
  const linearEnv = config.linear?.apiKeyEnv ?? "LINEAR_API_KEY";
  console.log(`${linearEnv}: ${process.env[linearEnv] ? "set" : "not set"}`);
  console.log(`interactive tty: ${canRunInteractive() ? "yes" : "no"}`);
}

async function cmdDebug(): Promise<void> {
  const {run} = await import("./core/process.js");
  console.log("tmux sessions:");
  try {
    const {stdout} = await run("tmux", ["list-sessions", "-F", "#{session_name}|created=#{session_created}|attached=#{session_attached}|windows=#{session_windows}"]);
    console.log(stdout.trim() || "<none>");
  } catch (error) {
    console.log(error instanceof Error ? error.message : String(error));
  }
  console.log("");
  console.log("tmux panes:");
  try {
    const {stdout} = await run("tmux", ["list-panes", "-a", "-F", "#{session_name}|#{window_index}.#{pane_index}|cmd=#{pane_current_command}|dead=#{pane_dead}|status=#{pane_dead_status}|path=#{pane_current_path}"]);
    console.log(stdout.trim() || "<none>");
  } catch (error) {
    console.log(error instanceof Error ? error.message : String(error));
  }
  console.log("");
  console.log("cmux agent state:");
  const sessions = listSessions();
  if (sessions.length === 0) {
    console.log("<none>");
  } else {
    for (const session of sessions) {
      console.log(`${session.id}|status=${session.status}|tmux=${session.tmuxSession}|workspace=${session.worktreePath ?? session.repoPath}`);
    }
  }
}

async function commandAvailable(command: string): Promise<boolean> {
  const {commandExists} = await import("./core/process.js");
  return commandExists(command);
}

function resolveSshHost(target: string): string {
  const configHome = process.env.XDG_CONFIG_HOME ?? resolve(homedir(), ".config");
  const hostsFile = resolve(configHome, "cmux", "hosts");
  if (!existsSync(hostsFile)) return target;
  const rows = readFileSync(hostsFile, "utf8").split("\n");
  for (const row of rows) {
    const trimmed = row.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const index = trimmed.indexOf("=");
    if (index === -1) continue;
    if (trimmed.slice(0, index) === target) return trimmed.slice(index + 1);
  }
  return target;
}

function printAgent(session: {id: string; status: string; title: string; tmuxSession: string; branch?: string; worktreePath?: string; repoPath: string; linear?: {url?: string}}): void {
  console.log(`${session.id}  ${session.status}  ${session.title}`);
  console.log(`tmux: ${session.tmuxSession}`);
  console.log(`branch: ${session.branch ?? "current"}`);
  console.log(`workspace: ${session.worktreePath ?? session.repoPath}`);
  if (session.linear?.url) console.log(`linear: ${session.linear.url}`);
}

function parseOptions(args: string[]): {positionals: string[]; options: Record<string, string>; flags: Set<string>} {
  const positionals: string[] = [];
  const options: Record<string, string> = {};
  const flags = new Set<string>();
  const booleanFlags = new Set(["m", "mobile", "no-attach", "worktree", "no-worktree", "prepare", "once"]);
  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index]!;
    if (arg.startsWith("--")) {
      const raw = arg.slice(2);
      const [key, inline] = raw.split("=", 2);
      if (booleanFlags.has(key)) {
        flags.add(key);
      } else if (inline !== undefined) {
        options[key] = inline;
      } else if (args[index + 1] && !args[index + 1]!.startsWith("-")) {
        options[key] = args[index + 1]!;
        index += 1;
      } else {
        flags.add(key);
      }
    } else if (arg.startsWith("-") && arg.length === 2) {
      const key = arg.slice(1);
      if (booleanFlags.has(key)) {
        flags.add(key);
      } else if (args[index + 1] && !args[index + 1]!.startsWith("-")) {
        options[key] = args[index + 1]!;
        index += 1;
      } else {
        flags.add(key);
      }
    } else {
      positionals.push(arg);
    }
  }
  return {positionals, options, flags};
}

async function execLegacy(args: string[]): Promise<void> {
  const dir = dirname(fileURLToPath(import.meta.url));
  const script = resolve(dir, "..", "legacy", "cmux.bash");
  await new Promise<void>((resolvePromise, reject) => {
    const child = spawn(script, args, {stdio: "inherit"});
    child.on("error", reject);
    child.on("exit", code => code === 0 ? resolvePromise() : reject(new Error(`legacy cmux exited with ${code}`)));
  });
}

function help(): void {
  console.log(`cmux v${version}

USAGE:
  cmux                         Interactive Ink selector
  cmux new [opts] [path]       Create a tmux agent session
  cmux list                    List tmux sessions
  cmux attach <name>           Attach to a session
  cmux switch                  Switch sessions inside tmux
  cmux agent <command>         Structured agent orchestration
  cmux daemon [--once]         Poll Linear auto-start rules
  cmux ssh <host|alias>        SSH to host and run cmux
  cmux doctor                  Check local cmux setup
  cmux debug                   Print tmux/cmux session diagnostics
  cmux legacy [command]        Run the preserved Bash implementation

AGENT COMMANDS:
  cmux agent start REB-123
  cmux agent start --title "Investigate build" --worktree
  cmux agent start REB-123 --prepare
  cmux agent scratch --title "Explore SDK bug"
  cmux agent list
  cmux agent open <id>
  cmux agent status <id> <status> [summary]
  cmux agent block <id> <reason>
  cmux agent review <id> [summary]
  cmux agent done <id> [summary]
  cmux agent promote <scratch-id> --issue REB-123
  cmux agent sync <id>
`);
}

function agentHelp(): void {
  console.log("Run 'cmux help' for agent command usage.");
}

main(process.argv.slice(2)).catch(error => {
  console.error(`Error: ${error instanceof Error ? error.message : String(error)}`);
  process.exitCode = 1;
});
