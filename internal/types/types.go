package types

type AgentSessionType string

const (
	TypeIssueBacked AgentSessionType = "issue-backed"
	TypeTaskBacked  AgentSessionType = "task-backed"
	TypeScratch     AgentSessionType = "scratch"
)

type AgentProfile string

const (
	ProfileGeneral   AgentProfile = "general"
	ProfilePlan      AgentProfile = "plan"
	ProfileImplement AgentProfile = "implement"
	ProfileDebug     AgentProfile = "debug"
	ProfileReview    AgentProfile = "review"
	ProfileCustom    AgentProfile = "custom"
)

type BriefKind string

const (
	BriefGeneral        BriefKind = "general"
	BriefPlan           BriefKind = "plan"
	BriefImplementation BriefKind = "implementation"
	BriefDebug          BriefKind = "debug"
	BriefReview         BriefKind = "review"
)

type AgentStatus string

const (
	StatusRunning        AgentStatus = "running"
	StatusStopped        AgentStatus = "stopped"
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
	IssueID         string `json:"issueId,omitempty"`
	Identifier      string `json:"identifier,omitempty"`
	URL             string `json:"url,omitempty"`
	CommentID       string `json:"commentId,omitempty"`
	State           string `json:"state,omitempty"`
	StateID         string `json:"stateId,omitempty"`
	OriginalState   string `json:"originalState,omitempty"`
	OriginalStateID string `json:"originalStateId,omitempty"`
	LastSyncAt      string `json:"lastSyncAt,omitempty"`
	LastSyncError   string `json:"lastSyncError,omitempty"`
}

type RuntimeData struct {
	LastScannedAt  string `json:"lastScannedAt,omitempty"`
	LastActivityAt string `json:"lastActivityAt,omitempty"`
	TmuxAlive      bool   `json:"tmuxAlive,omitempty"`
	PaneDead       bool   `json:"paneDead,omitempty"`
	ExitStatus     string `json:"exitStatus,omitempty"`
	CurrentCommand string `json:"currentCommand,omitempty"`
	Preview        string `json:"preview,omitempty"`
	GitDirty       bool   `json:"gitDirty,omitempty"`
	GitSummary     string `json:"gitSummary,omitempty"`
}

type BriefData struct {
	Kind                   BriefKind `json:"kind,omitempty"`
	SourcePath             string    `json:"sourcePath,omitempty"`
	PublishedHash          string    `json:"publishedHash,omitempty"`
	PublishedAt            string    `json:"publishedAt,omitempty"`
	PublishedLinearIssueID string    `json:"publishedLinearIssueId,omitempty"`
	PublishedBlockVersion  string    `json:"publishedBlockVersion,omitempty"`
	LastPublishError       string    `json:"lastPublishError,omitempty"`
	LastSyncAt             string    `json:"lastSyncAt,omitempty"`
	LastSyncError          string    `json:"lastSyncError,omitempty"`
	PublishedGitHead       string    `json:"publishedGitHead,omitempty"`
	PublishedGitDiffHash   string    `json:"publishedGitDiffHash,omitempty"`
	PublishedBranch        string    `json:"publishedBranch,omitempty"`
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
	Profile       AgentProfile     `json:"profile,omitempty"`
	Runtime       RuntimeData      `json:"runtime,omitempty"`
	Brief         BriefData        `json:"brief,omitempty"`
	Status        AgentStatus      `json:"status"`
	LastSummary   string           `json:"lastSummary,omitempty"`
	NeedsHuman    bool             `json:"needsHuman"`
	CreatedAt     string           `json:"createdAt"`
	LastUpdatedAt string           `json:"lastUpdatedAt"`
}

type LinearIssue struct {
	ID           string
	Identifier   string
	Title        string
	Description  string
	URL          string
	BranchName   string
	State        string
	StateID      string
	TeamID       string
	TeamKey      string
	TeamName     string
	Priority     int
	SortOrder    float64
	AssigneeID   string
	AssigneeName string
	Labels       []LinearLabel
}

type LinearViewer struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type LinearTeam struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

type LinearWorkflowState struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	TeamID string `json:"teamId"`
}

type LinearLabel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type QueuePreset struct {
	Name         string   `json:"name"`
	RepoPath     string   `json:"repoPath,omitempty"`
	Teams        []string `json:"teams,omitempty"`
	States       []string `json:"states,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	AssigneeMode string   `json:"assigneeMode,omitempty"`
	Priority     []int    `json:"priority,omitempty"`
	Limit        int      `json:"limit,omitempty"`
}

type RepoConfig struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type Config struct {
	DefaultAgent         string        `json:"defaultAgent,omitempty"`
	DefaultRepoPath      string        `json:"defaultRepoPath,omitempty"`
	DefaultEditorCommand string        `json:"defaultEditorCommand,omitempty"`
	DefaultSSHTarget     string        `json:"defaultSshTarget,omitempty"`
	Repos                []RepoConfig  `json:"repos,omitempty"`
	EditorCommands       []string      `json:"editorCommands,omitempty"`
	QueuePresets         []QueuePreset `json:"queuePresets,omitempty"`
	DefaultQueuePreset   string        `json:"defaultQueuePreset,omitempty"`
}
