package types

type AgentSessionType string

const (
	TypeIssueBacked AgentSessionType = "issue-backed"
	TypeTaskBacked  AgentSessionType = "task-backed"
	TypeScratch     AgentSessionType = "scratch"
)

type AgentStatus string

const (
	StatusRunning        AgentStatus = "running"
	StatusIdle           AgentStatus = "idle"
	StatusWaiting        AgentStatus = "waiting_for_input"
	StatusBlocked        AgentStatus = "blocked"
	StatusTestsFailed    AgentStatus = "tests_failed"
	StatusReadyForReview AgentStatus = "ready_for_review"
	StatusPROpened       AgentStatus = "pr_opened"
	StatusDone           AgentStatus = "done"
	StatusStale          AgentStatus = "stale"
	StatusCrashed        AgentStatus = "crashed"
)

type LinearData struct {
	IssueID    string `json:"issueId,omitempty"`
	Identifier string `json:"identifier,omitempty"`
	URL        string `json:"url,omitempty"`
	CommentID  string `json:"commentId,omitempty"`
	State      string `json:"state,omitempty"`
}

type AgentSession struct {
	SchemaVersion int              `json:"schemaVersion"`
	ID            string           `json:"id"`
	Type          AgentSessionType `json:"type"`
	Title         string           `json:"title"`
	Provider      string           `json:"provider"`
	AgentCommand  string           `json:"agentCommand"`
	TmuxSession   string           `json:"tmuxSession"`
	RepoPath      string           `json:"repoPath"`
	WorktreePath  string           `json:"worktreePath,omitempty"`
	Branch        string           `json:"branch,omitempty"`
	Linear        LinearData       `json:"linear,omitempty"`
	Status        AgentStatus      `json:"status"`
	LastSummary   string           `json:"lastSummary,omitempty"`
	NeedsHuman    bool             `json:"needsHuman"`
	CreatedAt     string           `json:"createdAt"`
	LastUpdatedAt string           `json:"lastUpdatedAt"`
}

type LinearIssue struct {
	ID          string
	Identifier  string
	Title       string
	Description string
	URL         string
	BranchName  string
	State       string
}
