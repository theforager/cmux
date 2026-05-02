import {loadConfig} from "./config.js";
import {readRunbookSummary} from "./runbook.js";
import type {AgentSession, AgentStatus, LinearIssue} from "./types.js";

async function client(): Promise<any> {
  const config = loadConfig();
  const envName = config.linear?.apiKeyEnv ?? "LINEAR_API_KEY";
  const apiKey = process.env[envName];
  if (!apiKey) {
    throw new Error(`Linear API key not found. Set ${envName} or configure linear.apiKeyEnv.`);
  }
  const mod = await import("@linear/sdk");
  return new mod.LinearClient({apiKey});
}

export async function getLinearIssue(identifier: string): Promise<LinearIssue> {
  const linear = await client();
  const issue = await linear.issue(identifier);
  if (!issue) throw new Error(`Linear issue not found: ${identifier}`);
  const state = await issue.state.catch?.(() => undefined) ?? await issue.state;
  const team = await issue.team.catch?.(() => undefined) ?? await issue.team;
  return {
    id: issue.id,
    identifier: issue.identifier,
    title: issue.title,
    description: issue.description ?? undefined,
    url: issue.url ?? undefined,
    branchName: issue.branchName ?? undefined,
    state: state?.name,
    teamId: team?.id
  };
}

export async function moveIssueToState(issue: LinearIssue | AgentSession, stateName?: string): Promise<void> {
  if (!stateName) return;
  const linear = await client();
  const issueId = "linear" in issue ? issue.linear?.issueId : issue.id;
  if (!issueId) return;
  const liveIssue = await linear.issue(issueId);
  const team = await liveIssue.team;
  const states = await team.states();
  const state = states.nodes.find((candidate: any) => candidate.name.toLowerCase() === stateName.toLowerCase());
  if (!state) throw new Error(`Linear state not found for team ${team.name}: ${stateName}`);
  await linear.updateIssue(issueId, {stateId: state.id});
}

export async function syncSessionToLinear(session: AgentSession): Promise<AgentSession> {
  const config = loadConfig();
  if (!session.linear) return session;
  const linear = await client();
  let next = session;
  if (config.linear?.syncStatus !== false) {
    const stateName = config.linear?.statusMap?.[session.status];
    if (stateName) await moveIssueToState(session, stateName);
  }
  if (config.linear?.managedComment !== false) {
    const body = managedCommentBody(session);
    if (session.linear.commentId) {
      await linear.updateComment(session.linear.commentId, {body});
    } else {
      const payload = await linear.createComment({issueId: session.linear.issueId, body});
      const comment = await payload.comment;
      if (comment?.id) {
        next = {...session, linear: {...session.linear, commentId: comment.id}};
      }
    }
  }
  return next;
}

export function managedCommentBody(session: AgentSession): string {
  const notes = readRunbookSummary(session.worktreePath ?? session.repoPath);
  const lines = [
    "cmux session",
    "",
    `Status: ${session.status}`,
    `Branch: ${session.branch ?? "current"}`,
    `Workspace: ${session.worktreePath ?? session.repoPath}`,
    `tmux: ${session.tmuxSession}`,
    session.lastSummary ? `Last summary: ${session.lastSummary}` : undefined,
    "",
    notes.current ? `Current state:\n${notes.current}` : undefined,
    notes.blockers ? `Blockers:\n${notes.blockers}` : undefined,
    notes.tests ? `Tests run:\n${notes.tests}` : undefined,
    notes.review ? `Review summary:\n${notes.review}` : undefined
  ];
  return lines.filter(Boolean).join("\n");
}

export async function listAutoStartIssues(): Promise<LinearIssue[]> {
  const config = loadConfig();
  const auto = config.linear?.autoStart;
  if (!auto?.enabled) return [];
  const linear = await client();
  const filter: any = {};
  if (auto.states?.length) filter.state = {name: {in: auto.states}};
  if (auto.teamKeys?.length) filter.team = {key: {in: auto.teamKeys}};
  if (auto.labels?.length) filter.labels = {some: {name: {in: auto.labels}}};
  const result = await linear.issues({first: 50, filter});
  const issues: LinearIssue[] = [];
  for (const issue of result.nodes) {
    const state = await issue.state;
    const team = await issue.team;
    issues.push({
      id: issue.id,
      identifier: issue.identifier,
      title: issue.title,
      description: issue.description ?? undefined,
      url: issue.url ?? undefined,
      branchName: issue.branchName ?? undefined,
      state: state?.name,
      teamId: team?.id
    });
  }
  return issues;
}

export function statusNeedsLinearNote(status: AgentStatus): boolean {
  return status === "blocked" || status === "tests_failed" || status === "ready_for_review" || status === "done";
}
