package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/theforager/cmux/internal/agent"
	"github.com/theforager/cmux/internal/config"
	"github.com/theforager/cmux/internal/format"
	"github.com/theforager/cmux/internal/gitx"
	"github.com/theforager/cmux/internal/home"
	"github.com/theforager/cmux/internal/linear"
	"github.com/theforager/cmux/internal/queue"
	"github.com/theforager/cmux/internal/state"
	"github.com/theforager/cmux/internal/tmux"
	"github.com/theforager/cmux/internal/tui"
	"github.com/theforager/cmux/internal/types"

	"github.com/spf13/cobra"
)

const version = "3.0.0-go"

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "cmux",
		Short:        "Terminal workbench for long-lived coding agents",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSelector(false)
		},
	}
	cmd.Version = version
	cmd.AddCommand(newCmd(), listCmd(), attachCmd(), switchCmd(), killCmd(), titleCmd(), infoCmd(), sshCmd(), debugCmd(), agentCmd(), queueCmd(), doctorCmd())
	return cmd
}

func newCmd() *cobra.Command {
	var title, agentCmd string
	var noAttach, mobile bool
	cmd := &cobra.Command{
		Use:   "new [path]",
		Short: "Create a tmux agent session",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			if agentCmd == "" {
				agentCmd = os.Getenv("CMUX_AGENT")
			}
			if agentCmd == "" {
				agentCmd = "claude"
			}
			name, err := tmux.GenerateSessionName(path, "")
			if err != nil {
				return err
			}
			if err := tmux.Create(tmux.CreateOptions{Name: name, Dir: path, Command: agentCmd, Title: title, Agent: agent.Provider(agentCmd), Mobile: mobile}); err != nil {
				return err
			}
			fmt.Printf("Created session: %s\n", name)
			if noAttach {
				return nil
			}
			return tmux.AttachOrSwitch(name)
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "session title")
	cmd.Flags().StringVarP(&agentCmd, "agent", "a", "", "agent command")
	cmd.Flags().BoolVarP(&mobile, "mobile", "m", false, "mobile-friendly width")
	cmd.Flags().BoolVar(&noAttach, "no-attach", false, "create without attaching")
	return cmd
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List tmux sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := tmux.List()
			if err != nil {
				return err
			}
			if len(sessions) == 0 {
				fmt.Println("No cmux sessions")
				return nil
			}
			for _, s := range sessions {
				status := tmux.InferStatus(s.Name)
				fmt.Printf("%-18s %-34s %s  %s\n", status, displayName(s), format.AgeUnix(s.Created), s.Name)
			}
			return nil
		},
	}
}

func attachCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "attach <name>",
		Aliases: []string{"a"},
		Short:   "Attach or switch to a session",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := tmux.Find(args[0])
			if err != nil {
				return err
			}
			return tmux.AttachOrSwitch(s.Name)
		},
	}
}

func switchCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "switch",
		Aliases: []string{"sw"},
		Short:   "Switch sessions inside tmux",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !tmux.Inside() {
				return fmt.Errorf("not inside tmux; use cmux instead")
			}
			return runSelector(true)
		},
	}
}

func killCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "kill <name>",
		Aliases: []string{"k"},
		Short:   "Kill a tmux session",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := tmux.Find(args[0])
			if err != nil {
				return err
			}
			if err := tmux.Kill(s.Name); err != nil {
				return err
			}
			fmt.Printf("Killed session: %s\n", s.Name)
			return nil
		},
	}
}

func titleCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "title <text>",
		Aliases: []string{"t"},
		Short:   "Set current session title",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			current, err := tmux.Current()
			if err != nil {
				return err
			}
			title := joinArgs(args)
			if err := tmux.SetEnv(current, "CMUX_TITLE", title); err != nil {
				return err
			}
			fmt.Printf("Title set: %s\n", title)
			return nil
		},
	}
}

func infoCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "info",
		Aliases: []string{"i"},
		Short:   "Show current session info",
		RunE: func(cmd *cobra.Command, args []string) error {
			current, err := tmux.Current()
			if err != nil {
				return err
			}
			sessions, _ := tmux.List()
			fmt.Println("Session:", current)
			for _, s := range sessions {
				if s.Name == current {
					fmt.Println("Directory:", s.Dir)
					fmt.Println("Title:", valueOr(s.Title, "<not set>"))
					fmt.Println("Agent:", valueOr(s.Agent, "claude"))
					return nil
				}
			}
			return nil
		},
	}
}

func sshCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "ssh <host|alias> [cmux-args...]",
		Short:              "SSH to a remote host and run cmux",
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = tmux.EnsureKeyOptions()
			hostArgs, err := resolveSSHHost(args[0])
			if err != nil {
				return err
			}
			sshArgs := append([]string{"ssh", "-t"}, hostArgs...)
			sshArgs = append(sshArgs, append([]string{"cmux"}, args[1:]...)...)
			path, err := exec.LookPath("ssh")
			if err != nil {
				return err
			}
			return syscall.Exec(path, sshArgs, os.Environ())
		},
	}
}

func debugCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "debug",
		Short: "Print tmux/cmux diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			return tmux.Debug(os.Stdout)
		},
	}
}

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local setup",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("cmux:", version)
			fmt.Println("tmux:", boolWord(tmux.Exists()))
			fmt.Println("home:", home.Dir())
			fmt.Println("config:", home.ConfigPath())
			fmt.Println("sessions:", home.SessionsDir())
			fmt.Println("worktrees:", home.WorktreesDir())
			if os.Getenv("LINEAR_API_KEY") == "" {
				fmt.Println("LINEAR_API_KEY: not set")
			} else {
				fmt.Println("LINEAR_API_KEY: set")
			}
			return nil
		},
	}
}

func agentCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "agent", Short: "Structured agent orchestration"}
	cmd.AddCommand(agentStartCmd(), agentScratchCmd(), agentListCmd(), agentOpenCmd(), agentPathCmd(), agentScanCmd(), agentRestartCmd(), agentCleanupCmd(), agentResetCmd(), agentNeedsReviewCmd(), agentScopedCmd(), agentAbandonCmd(), agentStatusCmd("status"), agentStatusCmd("block"), agentStatusCmd("review"), agentStatusCmd("done"))
	return cmd
}

func agentStartCmd() *cobra.Command {
	var title, agentCommand string
	var worktree, noWorktree, prepare, scoping bool
	cmd := &cobra.Command{
		Use:   "start [ISSUE]",
		Short: "Start a Linear issue-backed or task-backed agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			var issue string
			if len(args) > 0 {
				issue = args[0]
			}
			s, err := agent.Start(agent.StartOptions{Cwd: ".", IssueKey: issue, Title: title, Agent: agentCommand, Scoping: scoping, Worktree: worktree, NoWorktree: noWorktree, PrepareOnly: prepare})
			if err != nil {
				return err
			}
			printAgent(s)
			return nil
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "task title")
	cmd.Flags().StringVarP(&agentCommand, "agent", "a", "", "agent command")
	cmd.Flags().BoolVar(&worktree, "worktree", false, "create worktree for task")
	cmd.Flags().BoolVar(&noWorktree, "no-worktree", false, "do not create worktree for issue")
	cmd.Flags().BoolVar(&prepare, "prepare", false, "prepare state/worktree without launching")
	cmd.Flags().BoolVar(&scoping, "scope", false, "start a scoping session for a Linear issue")
	return cmd
}

func agentScratchCmd() *cobra.Command {
	var title, agentCommand string
	cmd := &cobra.Command{
		Use:   "scratch",
		Short: "Start a scratch agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := agent.Start(agent.StartOptions{Cwd: ".", Scratch: true, Title: title, Agent: agentCommand})
			if err != nil {
				return err
			}
			printAgent(s)
			return nil
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "Scratch session", "scratch title")
	cmd.Flags().StringVarP(&agentCommand, "agent", "a", "", "agent command")
	return cmd
}

func agentListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List structured agent sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := state.List()
			if err != nil {
				return err
			}
			if len(sessions) == 0 {
				fmt.Println("No cmux agent sessions")
				return nil
			}
			fmt.Printf("%-16s %-17s %-13s %-34s UPDATED  TITLE\n", "ID", "STATUS", "TYPE", "BRANCH")
			for _, s := range sessions {
				fmt.Printf("%-16s %-17s %-13s %-34s %-7s %s\n", format.Trunc(s.ID, 16), s.Status, s.Type, format.Trunc(valueOr(s.Branch, "current"), 34), format.Age(s.LastUpdatedAt), s.Title)
			}
			return nil
		},
	}
}

func agentOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "open <id>",
		Aliases: []string{"attach"},
		Short:   "Open a structured agent session",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := state.Read(args[0])
			if err == nil {
				return tmux.AttachOrSwitch(s.TmuxSession)
			}
			found, err := tmux.Find(args[0])
			if err != nil {
				return err
			}
			return tmux.AttachOrSwitch(found.Name)
		},
	}
}

func agentPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path <id>",
		Short: "Print a structured agent worktree path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := state.Read(args[0])
			if err != nil {
				return err
			}
			fmt.Println(valueOr(s.WorktreePath, s.RepoPath))
			return nil
		},
	}
}

func agentScanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Scan tmux/git runtime state for structured agent sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := agent.Scan()
			if err != nil {
				return err
			}
			fmt.Printf("scanned=%d updated=%d crashed=%d waiting=%d\n", result.Scanned, result.Updated, result.Crashed, result.Waiting)
			return nil
		},
	}
}

func agentRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart <id>",
		Short: "Restart a missing/dead structured agent tmux session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := agent.Restart(args[0])
			if err != nil {
				return err
			}
			printAgent(s)
			return nil
		},
	}
}

func agentCleanupCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "cleanup <id>",
		Short: "Remove a structured agent worktree if it is clean",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := agent.CleanupWorktree(args[0], force); err != nil {
				return err
			}
			fmt.Println("Cleaned up worktree and deleted session:", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "force cleanup dirty or external worktree")
	return cmd
}

func agentResetCmd() *cobra.Command {
	var confirm string
	cmd := &cobra.Command{
		Use:   "reset <id>",
		Short: "Reset a cmux-owned worktree with git reset --hard and git clean -fd",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if confirm != args[0] {
				return fmt.Errorf("refusing reset; pass --confirm %s", args[0])
			}
			if err := agent.ResetWorkspace(args[0]); err != nil {
				return err
			}
			fmt.Println("Reset workspace:", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&confirm, "confirm", "", "session id required to confirm destructive reset")
	return cmd
}

func queueCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Open the Linear worklist browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			selected, err := tui.RunQueue()
			if err != nil {
				return err
			}
			if selected == "" {
				return nil
			}
			return tmux.AttachOrSwitch(selected)
		},
	}
	cmd.AddCommand(queueListCmd(), queueSetupCmd())
	return cmd
}

func queueListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list [preset]",
		Short: "Print Linear worklist rows joined with cmux sessions",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			preset := ""
			if len(args) > 0 {
				preset = args[0]
			}
			if !queue.Configured() {
				fmt.Println("Linear worklist not configured: set LINEAR_API_KEY and run cmux queue setup")
				return nil
			}
			rows, used, err := queue.Rows(preset, 250)
			if err != nil {
				return err
			}
			fmt.Printf("Preset: %s\n", used.Name)
			if len(rows) == 0 {
				fmt.Println("No matching Linear issues")
				return nil
			}
			fmt.Printf("%-12s %-18s %-10s %-12s %-10s TITLE\n", "ISSUE", "STATUS", "TEAM", "LINEAR", "SESSION")
			for _, r := range rows {
				if r.Started {
					continue
				}
				session := ""
				if r.Session != nil {
					session = r.Session.ID
				}
				fmt.Printf("%-12s %-18s %-10s %-12s %-10s %s\n", r.Issue.Identifier, r.Status, valueOr(r.Issue.TeamKey, "-"), format.Trunc(valueOr(r.Issue.State, "-"), 12), format.Trunc(valueOr(session, "-"), 10), r.Issue.Title)
			}
			return nil
		},
	}
}

func queueSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Create a saved Linear worklist preset",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !queue.Configured() {
				return fmt.Errorf("LINEAR_API_KEY is not set")
			}
			viewer, err := linear.Viewer()
			if err != nil {
				return err
			}
			teams, err := linear.ListTeams()
			if err != nil {
				return err
			}
			states, err := linear.ListWorkflowStates()
			if err != nil {
				return err
			}
			labels, err := linear.ListLabels()
			if err != nil {
				return err
			}
			reader := bufio.NewReader(os.Stdin)
			fmt.Printf("Linear viewer: %s <%s>\n", viewer.Name, viewer.Email)
			name := prompt(reader, "Preset name", "Agent Ready")
			repoPath := prompt(reader, "Default repository path for this preset", defaultRepoPath())
			selectedTeams := chooseTeams(reader, teams)
			selectedStates := chooseStates(reader, states)
			selectedLabels := chooseLabels(reader, labels)
			assignee := prompt(reader, "Assignee mode (any/viewer/unassigned)", "unassigned")
			limitText := prompt(reader, "Limit", "8")
			limit, _ := strconv.Atoi(limitText)
			if limit <= 0 {
				limit = 8
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			preset := types.QueuePreset{Name: name, RepoPath: repoPath, Teams: selectedTeams, States: selectedStates, Labels: selectedLabels, AssigneeMode: assignee, Limit: limit}
			replaced := false
			for i := range cfg.QueuePresets {
				if cfg.QueuePresets[i].Name == name {
					cfg.QueuePresets[i] = preset
					replaced = true
				}
			}
			if !replaced {
				cfg.QueuePresets = append(cfg.QueuePresets, preset)
			}
			if cfg.DefaultQueuePreset == "" {
				cfg.DefaultQueuePreset = name
			}
			if cfg.DefaultRepoPath == "" {
				cfg.DefaultRepoPath = repoPath
			}
			cfg = config.AddRepo(cfg, types.RepoConfig{Name: filepath.Base(repoPath), Path: repoPath})
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Println("Saved preset:", name)
			return nil
		},
	}
}

func agentStatusCmd(kind string) *cobra.Command {
	use := kind + " <id> [summary]"
	return &cobra.Command{
		Use:   use,
		Short: "Update structured agent status",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			status := types.AgentStatus(kind)
			id := args[0]
			summary := ""
			if len(args) > 1 {
				summary = joinArgs(args[1:])
			}
			switch kind {
			case "block":
				status = types.StatusBlocked
			case "review":
				status = types.StatusReadyForReview
			case "done":
				status = types.StatusDone
			case "status":
				if len(args) < 2 {
					return fmt.Errorf("usage: cmux agent status <id> <status> [summary]")
				}
				status = types.AgentStatus(args[1])
				if len(args) > 2 {
					summary = joinArgs(args[2:])
				}
			}
			var s types.AgentSession
			var err error
			if kind == "done" {
				s, err = agent.Complete(id, summary)
			} else {
				s, err = agent.SetStatus(id, status, summary)
			}
			if err != nil {
				return err
			}
			printAgent(s)
			return nil
		},
	}
}

func agentScopedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scoped <id> [summary]",
		Short: "Mark a scoping session scoped and move the Linear issue to its ready state",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			summary := ""
			if len(args) > 1 {
				summary = joinArgs(args[1:])
			}
			s, err := agent.MarkScoped(args[0], summary)
			if err != nil {
				return err
			}
			printAgent(s)
			return nil
		},
	}
}

func agentNeedsReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "needs-review <id> [summary]",
		Short: "Promote a session to the Linear needs-review queue",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			summary := ""
			if len(args) > 1 {
				summary = joinArgs(args[1:])
			}
			s, err := agent.MarkNeedsReview(args[0], summary)
			if err != nil {
				return err
			}
			printAgent(s)
			return nil
		},
	}
}

func agentAbandonCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "abandon <id> [summary]",
		Short: "Abandon active work and move the Linear issue back to its original queue state",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			summary := ""
			if len(args) > 1 {
				summary = joinArgs(args[1:])
			}
			s, err := agent.Abandon(args[0], summary)
			if err != nil {
				return err
			}
			printAgent(s)
			return nil
		},
	}
}

func runSelector(popup bool) error {
	selected, err := tui.Run(popup)
	if err != nil {
		return err
	}
	if selected == "" {
		return nil
	}
	return tmux.AttachOrSwitch(selected)
}

func displayName(s tmux.Session) string {
	if s.Title != "" {
		return s.Title
	}
	return tmux.Child(s.Name)
}

func printAgent(s types.AgentSession) {
	fmt.Printf("%s  %s  %s\n", s.ID, s.Status, s.Title)
	if s.Phase != "" {
		fmt.Println("phase:", s.Phase)
	}
	fmt.Println("tmux:", s.TmuxSession)
	fmt.Println("branch:", valueOr(s.Branch, "current"))
	fmt.Println("workspace:", valueOr(s.WorktreePath, s.RepoPath))
	if s.Linear.URL != "" {
		fmt.Println("linear:", s.Linear.URL)
	}
}

func joinArgs(args []string) string {
	out := ""
	for i, arg := range args {
		if i > 0 {
			out += " "
		}
		out += arg
	}
	return out
}

func resolveSSHHost(target string) ([]string, error) {
	if strings.TrimSpace(target) == "" {
		return nil, fmt.Errorf("ssh host is required")
	}
	hostsPath := filepath.Join(configHome(), "cmux", "hosts")
	b, err := os.ReadFile(hostsPath)
	if err == nil {
		prefix := target + "="
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if strings.HasPrefix(line, prefix) {
				return strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, prefix))), nil
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return []string{target}, nil
}

func configHome() string {
	if value := os.Getenv("XDG_CONFIG_HOME"); strings.TrimSpace(value) != "" {
		return value
	}
	return filepath.Join(homePath(), ".config")
}

func homePath() string {
	if value := os.Getenv("HOME"); strings.TrimSpace(value) != "" {
		return value
	}
	return "."
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func boolWord(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func prompt(reader *bufio.Reader, label, fallback string) string {
	fmt.Printf("%s [%s]: ", label, fallback)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return fallback
	}
	return line
}

func defaultRepoPath() string {
	wd, _ := os.Getwd()
	if root, err := gitx.Root(wd); err == nil && root != "" {
		return root
	}
	return wd
}

func chooseTeams(reader *bufio.Reader, teams []types.LinearTeam) []string {
	if len(teams) == 0 {
		return nil
	}
	items := make([]tui.ChecklistItem, 0, len(teams))
	for _, team := range teams {
		items = append(items, tui.ChecklistItem{ID: team.ID, Label: team.Name, Description: team.Key})
	}
	selected, err := tui.RunChecklist(tui.ChecklistOptions{Title: "Linear Teams", Help: "Leave empty to include all teams.", Items: items})
	if err != nil {
		fmt.Println("Team setup skipped:", err)
		return nil
	}
	return selected
}

func chooseStates(reader *bufio.Reader, states []types.LinearWorkflowState) []string {
	if len(states) == 0 {
		return nil
	}
	items := make([]tui.ChecklistItem, 0, len(states))
	for _, state := range states {
		items = append(items, tui.ChecklistItem{ID: state.ID, Label: state.Name, Description: state.Type})
	}
	selected, err := tui.RunChecklist(tui.ChecklistOptions{
		Title:    "Linear Workflow States",
		Help:     "Check states in worklist order. Empty uses In Progress -> Todo -> Scoping -> Backlog.",
		Items:    items,
		Selected: defaultStateIDs(states),
		Ordered:  true,
	})
	if err != nil {
		fmt.Println("State setup skipped:", err)
		return nil
	}
	return selected
}

func chooseLabels(reader *bufio.Reader, labels []types.LinearLabel) []string {
	if len(labels) == 0 {
		return nil
	}
	items := make([]tui.ChecklistItem, 0, len(labels))
	for _, label := range labels {
		items = append(items, tui.ChecklistItem{ID: label.ID, Label: label.Name})
	}
	selected, err := tui.RunChecklist(tui.ChecklistOptions{Title: "Linear Labels", Help: "Leave empty to include all labels.", Items: items})
	if err != nil {
		fmt.Println("Label setup skipped:", err)
		return nil
	}
	return selected
}

func defaultStateIDs(states []types.LinearWorkflowState) []string {
	wanted := []string{"in progress", "todo", "to do", "scoping", "backlog"}
	out := []string{}
	seen := map[string]bool{}
	for _, want := range wanted {
		for _, state := range states {
			if seen[state.ID] {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(state.Name), want) {
				out = append(out, state.ID)
				seen[state.ID] = true
				break
			}
		}
	}
	return out
}
