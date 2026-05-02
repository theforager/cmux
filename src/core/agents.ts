import {existsSync} from "node:fs";
import {resolve} from "node:path";
import {loadConfig} from "./config.js";
import {assert} from "./errors.js";
import {fallbackRepoPath, gitRoot, ensureWorktree} from "./git.js";
import {runStatusHook} from "./hooks.js";
import {getLinearIssue, moveIssueToState, syncSessionToLinear} from "./linear.js";
import {commandExists, isExecutable} from "./process.js";
import {ensureRunbook} from "./runbook.js";
import {
  attach,
  createSession,
  findSession,
  generateSessionName,
  getCurrentSession,
  kill,
  pastePrompt,
  renameSession,
  setEnvironment
} from "./tmux.js";
import {nowIso, slugify} from "./format.js";
import {nextScratchId, readSession, setSessionStatus, updateSession, writeSession} from "./state.js";
import type {AgentProvider, AgentSession, AgentStatus, LinearIssue, StartAgentOptions} from "./types.js";

export function providerFor(agentCommand: string): AgentProvider {
  const first = agentCommand.trim().split(/\s+/)[0]?.toLowerCase();
  if (first === "claude") return "claude";
  if (first === "codex") return "codex";
  return "custom";
}

export async function validateAgentCommand(agentCommand: string): Promise<void> {
  const first = agentCommand.trim().split(/\s+/)[0];
  assert(first, "Agent command cannot be empty");
  if (first.startsWith("/")) {
    assert(existsSync(first) && isExecutable(first), `Agent command not executable: ${first}`);
  } else {
    assert(await commandExists(first), `Agent command not found in PATH: ${first}`);
  }
}

export async function startAgent(options: StartAgentOptions): Promise<AgentSession> {
  const config = loadConfig();
  const agentCommand = options.agent ?? config.defaultAgent ?? process.env.CMUX_AGENT ?? "claude";
  await validateAgentCommand(agentCommand);

  if (options.scratch) {
    return startScratch({...options, agent: agentCommand});
  }

  if (options.issueKey) {
    return startIssueBacked({...options, agent: agentCommand}, config.linear?.moveToStateOnStart);
  }

  assert(options.title, "Use --title for task-backed sessions without a Linear issue");
  return startTaskBacked({...options, agent: agentCommand});
}

async function startIssueBacked(options: StartAgentOptions, startState?: string): Promise<AgentSession> {
  assert(options.issueKey, "Missing Linear issue identifier");
  const issue = await getLinearIssue(options.issueKey);
  const existing = readSession(issue.identifier);
  if (existing) return existing;

  const workspace = options.noWorktree
    ? {
        repoPath: await repoPathOrCwd(options.cwd),
        branch: undefined,
        worktreePath: await repoPathOrCwd(options.cwd)
      }
    : await ensureWorktree({
        cwd: options.cwd,
        identifier: issue.identifier,
        title: issue.title,
        branchName: issue.branchName
      });

  const tmuxSession = await generateSessionName(workspace.worktreePath, issue.identifier);
  const now = nowIso();
  let session: AgentSession = {
    schemaVersion: 1,
    id: issue.identifier,
    type: "issue-backed",
    title: issue.title,
    provider: providerFor(options.agent!),
    agentCommand: options.agent!,
    tmuxSession,
    repoPath: workspace.repoPath,
    worktreePath: workspace.worktreePath,
    branch: workspace.branch,
    linear: {
      issueId: issue.id,
      identifier: issue.identifier,
      url: issue.url,
      state: issue.state
    },
    status: options.prepareOnly ? "idle" : "running",
    needsHuman: false,
    createdAt: now,
    lastUpdatedAt: now
  };

  ensureRunbook(workspace.worktreePath, session, issue);
  writeSession(session);

  if (!options.prepareOnly) {
    await launchSession(session, issue);
  }
  if (startState) await moveIssueToState(issue, startState);
  session = await syncSessionToLinear(session);
  writeSession(session);
  return session;
}

async function startTaskBacked(options: StartAgentOptions): Promise<AgentSession> {
  assert(options.title, "Missing task title");
  const id = `task-${slugify(options.title)}`;
  const existing = readSession(id);
  if (existing) return existing;
  const useWorktree = options.worktree === true;
  const workspace = useWorktree
    ? await ensureWorktree({cwd: options.cwd, identifier: id, title: options.title})
    : {repoPath: await repoPathOrCwd(options.cwd), branch: undefined, worktreePath: resolve(options.cwd)};
  const tmuxSession = await generateSessionName(workspace.worktreePath, id);
  const now = nowIso();
  const session: AgentSession = {
    schemaVersion: 1,
    id,
    type: "task-backed",
    title: options.title,
    provider: providerFor(options.agent!),
    agentCommand: options.agent!,
    tmuxSession,
    repoPath: workspace.repoPath,
    worktreePath: workspace.worktreePath,
    branch: workspace.branch,
    status: options.prepareOnly ? "idle" : "running",
    needsHuman: false,
    createdAt: now,
    lastUpdatedAt: now
  };
  ensureRunbook(workspace.worktreePath, session);
  writeSession(session);
  if (!options.prepareOnly) await launchSession(session);
  return session;
}

async function startScratch(options: StartAgentOptions): Promise<AgentSession> {
  const id = nextScratchId();
  const cwd = resolve(options.cwd);
  const tmuxSession = await generateSessionName(cwd, id);
  const now = nowIso();
  const session: AgentSession = {
    schemaVersion: 1,
    id,
    type: "scratch",
    title: options.title ?? "Scratch session",
    provider: providerFor(options.agent!),
    agentCommand: options.agent!,
    tmuxSession,
    repoPath: await repoPathOrCwd(cwd),
    worktreePath: cwd,
    status: "running",
    needsHuman: false,
    createdAt: now,
    lastUpdatedAt: now
  };
  writeSession(session);
  await launchSession(session);
  return session;
}

export async function launchSession(session: AgentSession, issue?: LinearIssue): Promise<void> {
  await createSession({
    name: session.tmuxSession,
    cwd: session.worktreePath ?? session.repoPath,
    command: session.agentCommand,
    title: session.title,
    agent: session.provider,
    metadata: {
      CMUX_AGENT_ID: session.id,
      CMUX_AGENT_TYPE: session.type,
      CMUX_LINEAR_ISSUE_ID: session.linear?.identifier
    }
  });
  if (session.type !== "scratch") {
    await new Promise(resolve => setTimeout(resolve, 900));
    await pastePrompt(session.tmuxSession, renderInitialPrompt(session, issue));
  }
}

function renderInitialPrompt(session: AgentSession, issue?: LinearIssue): string {
  const lines = [
    `You are working in a cmux managed ${session.type} session.`,
    "",
    issue ? `Linear issue: ${issue.identifier} - ${issue.title}` : `Task: ${session.title}`,
    session.branch ? `Branch: ${session.branch}` : undefined,
    `Workspace: ${session.worktreePath ?? session.repoPath}`,
    "",
    "Requirements:",
    "- Work only in this workspace unless the user explicitly says otherwise.",
    "- Keep .agent/RUNBOOK.md updated with current state, decisions, blockers, tests run, next action, and review summary.",
    `- When blocked, run: cmux agent status ${session.id} blocked \"<reason>\"`,
    `- When tests fail and need human attention, run: cmux agent status ${session.id} tests_failed \"<summary>\"`,
    `- When ready for review, run: cmux agent status ${session.id} ready_for_review \"<summary>\"`,
    "- Before marking ready, run the relevant tests when feasible and record them in the runbook.",
    "",
    issue?.description ? `Issue description:\n${issue.description}` : undefined
  ];
  return lines.filter(Boolean).join("\n");
}

export async function openAgent(id: string): Promise<void> {
  const session = readSession(id);
  if (session) {
    await attach(session.tmuxSession);
    return;
  }
  const tmuxSession = await findSession(id);
  assert(tmuxSession, `No session matching: ${id}`);
  await attach(tmuxSession.name);
}

export async function updateAgentStatus(id: string, status: AgentStatus, summary?: string): Promise<AgentSession> {
  const {previous, next} = setSessionStatus(id, status, summary);
  await runStatusHook(previous, next);
  const synced = await syncSessionToLinear(next);
  if (synced !== next) {
    return updateSession(id, () => synced);
  }
  return next;
}

export async function promoteScratch(id: string, issueKey: string): Promise<AgentSession> {
  const session = readSession(id);
  assert(session, `No agent session found: ${id}`);
  assert(session.type === "scratch", "Only scratch sessions can be promoted");
  const issue = await getLinearIssue(issueKey);
  const promoted: AgentSession = {
    ...session,
    id: issue.identifier,
    type: "issue-backed",
    title: issue.title,
    linear: {
      issueId: issue.id,
      identifier: issue.identifier,
      url: issue.url,
      state: issue.state
    },
    lastUpdatedAt: nowIso()
  };
  await setEnvironment(promoted.tmuxSession, "CMUX_AGENT_ID", promoted.id);
  await setEnvironment(promoted.tmuxSession, "CMUX_AGENT_TYPE", promoted.type);
  await setEnvironment(promoted.tmuxSession, "CMUX_LINEAR_ISSUE_ID", issue.identifier);
  ensureRunbook(promoted.worktreePath ?? promoted.repoPath, promoted, issue);
  let synced = await syncSessionToLinear(promoted);
  writeSession(synced);
  return synced;
}

export async function killAgent(id: string): Promise<void> {
  const session = readSession(id);
  if (session) {
    await kill(session.tmuxSession);
    await updateAgentStatus(id, "done", "Session killed");
    return;
  }
  const tmuxSession = await findSession(id);
  assert(tmuxSession, `No session matching: ${id}`);
  await kill(tmuxSession.name);
}

export async function renameCurrentSession(name: string): Promise<void> {
  const current = await getCurrentSession();
  const next = await renameSession(current, name);
  await setEnvironment(next, "CMUX_SESSION", next);
}

async function repoPathOrCwd(cwd: string): Promise<string> {
  try {
    return await gitRoot(cwd);
  } catch {
    return fallbackRepoPath(resolve(cwd));
  }
}
