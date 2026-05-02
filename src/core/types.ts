export type AgentSessionType = "issue-backed" | "task-backed" | "scratch";

export type AgentProvider = "claude" | "codex" | "custom";

export type AgentStatus =
  | "running"
  | "idle"
  | "waiting_for_input"
  | "blocked"
  | "tests_failed"
  | "ready_for_review"
  | "pr_opened"
  | "done"
  | "stale"
  | "crashed";

export const agentStatuses: AgentStatus[] = [
  "running",
  "idle",
  "waiting_for_input",
  "blocked",
  "tests_failed",
  "ready_for_review",
  "pr_opened",
  "done",
  "stale",
  "crashed"
];

export interface LinearSessionData {
  issueId: string;
  identifier: string;
  url?: string;
  commentId?: string;
  state?: string;
}

export interface AgentSession {
  schemaVersion: 1;
  id: string;
  type: AgentSessionType;
  title: string;
  provider: AgentProvider;
  agentCommand: string;
  tmuxSession: string;
  repoPath: string;
  worktreePath?: string;
  branch?: string;
  linear?: LinearSessionData;
  status: AgentStatus;
  lastSummary?: string;
  needsHuman: boolean;
  createdAt: string;
  lastUpdatedAt: string;
}

export interface CmuxConfig {
  stateDir?: string;
  worktreeRoot?: string;
  defaultAgent?: string;
  linear?: {
    apiKeyEnv?: string;
    moveToStateOnStart?: string;
    commentOnStart?: boolean;
    syncStatus?: boolean;
    managedComment?: boolean;
    statusMap?: Partial<Record<AgentStatus, string>>;
    autoStart?: {
      enabled?: boolean;
      mode?: "prepare" | "start";
      intervalSeconds?: number;
      teamKeys?: string[];
      states?: string[];
      labels?: string[];
      maxConcurrent?: number;
    };
  };
}

export interface LinearIssue {
  id: string;
  identifier: string;
  title: string;
  description?: string;
  url?: string;
  branchName?: string;
  state?: string;
  teamId?: string;
}

export interface StartAgentOptions {
  issueKey?: string;
  title?: string;
  scratch?: boolean;
  worktree?: boolean;
  noWorktree?: boolean;
  prepareOnly?: boolean;
  agent?: string;
  cwd: string;
}
