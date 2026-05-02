import {existsSync, mkdirSync, readFileSync, writeFileSync} from "node:fs";
import {join} from "node:path";
import type {AgentSession, LinearIssue} from "./types.js";

export function runbookPath(workspace: string): string {
  return join(workspace, ".agent", "RUNBOOK.md");
}

export function workspaceMetaPath(workspace: string): string {
  return join(workspace, ".agent", "cmux.json");
}

export function ensureRunbook(workspace: string, session: AgentSession, issue?: LinearIssue): void {
  const dir = join(workspace, ".agent");
  mkdirSync(dir, {recursive: true});
  const path = runbookPath(workspace);
  if (!existsSync(path)) {
    writeFileSync(path, renderRunbook(session, issue));
  }
  writeFileSync(workspaceMetaPath(workspace), `${JSON.stringify({sessionId: session.id, tmuxSession: session.tmuxSession}, null, 2)}\n`);
}

function renderRunbook(session: AgentSession, issue?: LinearIssue): string {
  return `# Agent Runbook

## Goal
${issue ? `${issue.identifier}: ${issue.title}` : session.title}

## Current state
Session created by cmux. Fill this in as work progresses.

## Decisions made
- None yet.

## Blockers
- None.

## Tests run
- Not run yet.

## Next action
- Start implementation.

## Review summary
- Not ready for review yet.
`;
}

export function readRunbookSummary(workspace?: string): {blockers?: string; tests?: string; review?: string; current?: string} {
  if (!workspace) return {};
  const path = runbookPath(workspace);
  if (!existsSync(path)) return {};
  const text = readFileSync(path, "utf8");
  return {
    current: section(text, "Current state"),
    blockers: section(text, "Blockers"),
    tests: section(text, "Tests run"),
    review: section(text, "Review summary")
  };
}

function section(text: string, heading: string): string | undefined {
  const escaped = heading.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const match = text.match(new RegExp(`## ${escaped}\\n([\\s\\S]*?)(?=\\n## |$)`));
  return match?.[1]?.trim();
}
