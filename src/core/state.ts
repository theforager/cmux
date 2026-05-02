import {existsSync, readdirSync, readFileSync, renameSync, writeFileSync} from "node:fs";
import {join} from "node:path";
import {ensureStateDirs, getAgentsDir, loadConfig} from "./config.js";
import {nowIso} from "./format.js";
import type {AgentSession, AgentStatus} from "./types.js";

function sessionPath(id: string): string {
  const config = loadConfig();
  ensureStateDirs(config);
  return join(getAgentsDir(config), `${id}.json`);
}

export function readSession(id: string): AgentSession | undefined {
  const path = sessionPath(id);
  if (!existsSync(path)) return undefined;
  return JSON.parse(readFileSync(path, "utf8")) as AgentSession;
}

export function listSessions(): AgentSession[] {
  const config = loadConfig();
  ensureStateDirs(config);
  return readdirSync(getAgentsDir(config))
    .filter(file => file.endsWith(".json"))
    .map(file => JSON.parse(readFileSync(join(getAgentsDir(config), file), "utf8")) as AgentSession)
    .sort((a, b) => b.lastUpdatedAt.localeCompare(a.lastUpdatedAt));
}

export function writeSession(session: AgentSession): AgentSession {
  const path = sessionPath(session.id);
  const tmp = `${path}.tmp`;
  writeFileSync(tmp, `${JSON.stringify(session, null, 2)}\n`);
  renameSync(tmp, path);
  return session;
}

export function updateSession(id: string, updater: (session: AgentSession) => AgentSession): AgentSession {
  const current = readSession(id);
  if (!current) throw new Error(`No agent session found: ${id}`);
  const next = updater(current);
  next.lastUpdatedAt = nowIso();
  return writeSession(next);
}

export function setSessionStatus(id: string, status: AgentStatus, summary?: string): {previous: AgentSession; next: AgentSession} {
  const previous = readSession(id);
  if (!previous) throw new Error(`No agent session found: ${id}`);
  const next: AgentSession = {
    ...previous,
    status,
    needsHuman: status === "blocked" || status === "tests_failed" || status === "ready_for_review",
    lastSummary: summary ?? previous.lastSummary,
    lastUpdatedAt: nowIso()
  };
  writeSession(next);
  return {previous, next};
}

export function nextScratchId(): string {
  const stamp = new Date().toISOString().slice(0, 10).replaceAll("-", "");
  const sessions = listSessions().map(session => session.id);
  let counter = 1;
  while (sessions.includes(`scratch-${stamp}-${counter}`)) counter += 1;
  return `scratch-${stamp}-${counter}`;
}
