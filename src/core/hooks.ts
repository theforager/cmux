import {existsSync} from "node:fs";
import {join} from "node:path";
import {getHooksDir} from "./config.js";
import {run} from "./process.js";
import type {AgentSession} from "./types.js";

export async function runStatusHook(previous: AgentSession, next: AgentSession): Promise<void> {
  if (previous.status === next.status) return;
  const hook = join(getHooksDir(), "on-status-change");
  if (!existsSync(hook)) return;
  const env = [
    `CMUX_SESSION_ID=${shellValue(next.id)}`,
    `CMUX_OLD_STATUS=${shellValue(previous.status)}`,
    `CMUX_NEW_STATUS=${shellValue(next.status)}`,
    `CMUX_LINEAR_ISSUE_ID=${shellValue(next.linear?.identifier ?? "")}`,
    `CMUX_WORKTREE=${shellValue(next.worktreePath ?? next.repoPath)}`
  ].join(" ");
  await run("sh", ["-lc", `${env} "$1"`, "sh", hook]);
}

function shellValue(value: string): string {
  return `'${value.replaceAll("'", "'\\''")}'`;
}
