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
	"github.com/theforager/cmux/internal/brief"
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
	cmd.AddCommand(newCmd(), listCmd(), inventoryCmd(), attachCmd(), switchCmd(), killCmd(), titleCmd(), infoCmd(), sshCmd(), debugCmd(), agentCmd(), briefCmd(), linearCmd(), sessionCmd(), queueCmd(), doctorCmd())
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

func inventoryCmd() *cobra.Command {
	var scan bool
	cmd := &cobra.Command{
		Use:     "inventory",
		Aliases: []string{"inv"},
		Short:   "Print a local inventory of cmux sessions and worktrees",
		RunE: func(cmd *cobra.Command, args []string) error {
			if scan {
				if _, err := agent.ScanWithOptions(agent.ScanOptions{RefreshLinear: false}); err != nil {
					return err
				}
			}
			sessions, err := state.List()
			if err != nil {
				return err
			}
			tmuxSessions, err := tmux.List()
			if err != nil {
				fmt.Fprintln(os.Stderr, "warning:", err)
				tmuxSessions = nil
			}
			active := map[string]tmux.Session{}
			for _, s := range tmuxSessions {
				active[s.Name] = s
			}
			seen := map[string]bool{}
			fmt.Printf("%-16s %-17s %-13s %-8s %-8s %-26s %-34s %s\n", "ID", "STATUS", "TYPE", "TMUX", "GIT", "BRANCH", "WORKSPACE", "TITLE")
			for _, s := range sessions {
				tmuxState := "down"
				if _, ok := active[s.TmuxSession]; ok {
					tmuxState = "up"
					seen[s.TmuxSession] = true
				}
				workspace := valueOr(s.WorktreePath, s.RepoPath)
				fmt.Printf("%-16s %-17s %-13s %-8s %-8s %-26s %-34s %s\n",
					format.Trunc(s.ID, 16),
					format.Trunc(string(s.Status), 17),
					format.Trunc(inventoryType(s), 13),
					tmuxState,
					inventoryGitState(s),
					format.Trunc(valueOr(s.Branch, "current"), 26),
					format.Trunc(valueOr(workspace, "-"), 34),
					s.Title,
				)
			}
			for _, s := range tmuxSessions {
				if seen[s.Name] {
					continue
				}
				fmt.Printf("%-16s %-17s %-13s %-8s %-8s %-26s %-34s %s\n",
					format.Trunc(tmux.Child(s.Name), 16),
					format.Trunc(tmux.InferStatus(s.Name), 17),
					"tmux",
					"up",
					inventoryPathGitState(s.Dir),
					"-",
					format.Trunc(valueOr(s.Dir, "-"), 34),
					displayName(s),
				)
			}
			if len(sessions) == 0 && len(tmuxSessions) == 0 {
				fmt.Println("No cmux sessions")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&scan, "scan", false, "refresh local runtime state before printing")
	return cmd
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
	cmd.AddCommand(agentStartCmd(), agentFreshCmd(), agentScratchCmd(), agentListCmd(), agentOpenCmd(), agentPathCmd(), agentScanCmd(), agentRestartCmd(), agentStopCmd(), agentStatusCmd("status"), agentStatusCmd("block"))
	return cmd
}

func agentStartCmd() *cobra.Command {
	var title, agentCommand, profile string
	var worktree, noWorktree, prepare bool
	cmd := &cobra.Command{
		Use:   "start [ISSUE]",
		Short: "Start a Linear issue-backed or task-backed agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			var issue string
			if len(args) > 0 {
				issue = args[0]
			}
			s, err := agent.Start(agent.StartOptions{Cwd: ".", IssueKey: issue, Title: title, Agent: agentCommand, AgentSet: cmd.Flags().Changed("agent"), Profile: parseProfile(profile), ProfileSet: cmd.Flags().Changed("profile"), Worktree: worktree, NoWorktree: noWorktree, PrepareOnly: prepare})
			if err != nil {
				return err
			}
			printAgent(s)
			return nil
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "task title")
	cmd.Flags().StringVarP(&agentCommand, "agent", "a", "", "agent command")
	cmd.Flags().StringVar(&profile, "profile", string(types.ProfileGeneral), "agent profile: general, plan, implement, debug, review, custom")
	cmd.Flags().BoolVar(&worktree, "worktree", false, "create worktree for task")
	cmd.Flags().BoolVar(&noWorktree, "no-worktree", false, "do not create worktree for issue")
	cmd.Flags().BoolVar(&prepare, "prepare", false, "prepare state/worktree without launching")
	return cmd
}

func agentFreshCmd() *cobra.Command {
	var agentCommand, profile string
	cmd := &cobra.Command{
		Use:   "fresh <ISSUE>",
		Short: "Start a fresh agent with the selected profile and current context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var s types.AgentSession
			if _, err := state.Read(args[0]); err == nil {
				s, err = agent.Fresh(args[0], agentCommand, parseProfile(profile))
				if err != nil {
					return err
				}
			} else if os.IsNotExist(err) {
				s, err = agent.Start(agent.StartOptions{Cwd: ".", IssueKey: args[0], Agent: agentCommand, AgentSet: cmd.Flags().Changed("agent"), Profile: parseProfile(profile), ProfileSet: cmd.Flags().Changed("profile"), Fresh: true})
				if err != nil {
					return err
				}
			} else {
				return err
			}
			printAgent(s)
			return nil
		},
	}
	cmd.Flags().StringVarP(&agentCommand, "agent", "a", "", "agent command")
	cmd.Flags().StringVar(&profile, "profile", string(types.ProfileImplement), "agent profile: general, plan, implement, debug, review, custom")
	return cmd
}

func agentScratchCmd() *cobra.Command {
	var title, agentCommand, profile string
	cmd := &cobra.Command{
		Use:   "scratch",
		Short: "Start a scratch agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := agent.Start(agent.StartOptions{Cwd: ".", Scratch: true, Title: title, Agent: agentCommand, AgentSet: cmd.Flags().Changed("agent"), Profile: parseProfile(profile), ProfileSet: cmd.Flags().Changed("profile")})
			if err != nil {
				return err
			}
			printAgent(s)
			return nil
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "Scratch session", "scratch title")
	cmd.Flags().StringVarP(&agentCommand, "agent", "a", "", "agent command")
	cmd.Flags().StringVar(&profile, "profile", string(types.ProfileGeneral), "agent profile: general, plan, implement, debug, review, custom")
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

func agentStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <id>",
		Short: "Stop an agent tmux session without closing the cmux session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := agent.Kill(args[0]); err != nil {
				return err
			}
			fmt.Println("Stopped agent:", args[0])
			return nil
		},
	}
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
			case "status":
				if len(args) < 2 {
					return fmt.Errorf("usage: cmux agent status <id> <status> [summary]")
				}
				status = types.AgentStatus(args[1])
				if len(args) > 2 {
					summary = joinArgs(args[2:])
				}
			}
			s, err := agent.SetStatus(id, status, summary)
			if err != nil {
				return err
			}
			printAgent(s)
			return nil
		},
	}
}

func briefCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "brief", Short: "Preview and publish the session brief"}
	cmd.AddCommand(briefOpenCmd(), briefPreviewCmd(), briefPublishCmd(), briefDiffCmd())
	return cmd
}

func briefOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open <id>",
		Short: "Print the session brief path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := state.Read(args[0])
			if err != nil {
				return err
			}
			fmt.Println(valueOr(s.Brief.SourcePath, home.BriefPath(s.ID)))
			return nil
		},
	}
}

func briefPreviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "preview <id>",
		Short: "Render the session brief",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rendered, err := agent.PreviewBrief(args[0])
			if err != nil {
				return err
			}
			fmt.Println(rendered)
			return nil
		},
	}
}

func briefPublishCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "publish <id>",
		Short: "Publish the session brief to Linear without moving status or closing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, hash, err := agent.PublishBrief(args[0])
			if err != nil {
				return err
			}
			fmt.Println("Published brief:", hash)
			return nil
		},
	}
}

func briefDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <id>",
		Short: "Show brief publication state and hashes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := state.Read(args[0])
			if err != nil {
				return err
			}
			rendered, err := agent.PreviewBrief(args[0])
			if err != nil {
				return err
			}
			fmt.Println("state:", brief.State(s))
			fmt.Println("current:", brief.Hash(rendered))
			fmt.Println("published:", valueOr(s.Brief.PublishedHash, "-"))
			return nil
		},
	}
}

func linearCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "linear", Short: "Linear synchronization and explicit status moves"}
	cmd.AddCommand(linearMoveCmd())
	return cmd
}

func linearMoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "move <id> <state>",
		Short: "Move a Linear issue to an explicitly selected workflow state name or id",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := agent.MoveLinear(args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Println("Linear:", valueOr(s.Linear.State, s.Linear.StateID))
			return nil
		},
	}
}

func sessionCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "session", Short: "Close or forget local cmux sessions"}
	cmd.AddCommand(sessionCloseCmd(), sessionForgetCmd())
	return cmd
}

func sessionCloseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "close <id>",
		Short: "Safely close a local session without changing Linear",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return agent.Close(args[0])
		},
	}
}

func sessionForgetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "forget <id>",
		Short: "Forget a local session without deleting the workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return agent.Delete(args[0])
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

func inventoryType(s types.AgentSession) string {
	base := string(s.Type)
	if s.Profile == "" || s.Profile == types.ProfileGeneral {
		return base
	}
	return base + "/" + string(s.Profile)
}

func inventoryGitState(s types.AgentSession) string {
	workspace := valueOr(s.WorktreePath, s.RepoPath)
	if workspace == "" {
		return "-"
	}
	if _, err := os.Stat(workspace); err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "error"
	}
	if s.RepoPath != "" && s.WorktreePath != "" && s.RepoPath != s.WorktreePath && !gitx.WorktreeListed(s.RepoPath, s.WorktreePath) {
		return "unlisted"
	}
	return inventoryPathGitState(workspace)
}

func inventoryPathGitState(path string) string {
	if strings.TrimSpace(path) == "" {
		return "-"
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "error"
	}
	dirty, summary := gitx.StatusSummary(path)
	if dirty {
		return "dirty"
	}
	if summary == "clean" {
		return "clean"
	}
	return "-"
}

func printAgent(s types.AgentSession) {
	fmt.Printf("%s  %s  %s\n", s.ID, s.Status, s.Title)
	if s.Profile != "" {
		fmt.Println("profile:", s.Profile)
	}
	fmt.Println("tmux:", s.TmuxSession)
	fmt.Println("branch:", valueOr(s.Branch, "current"))
	fmt.Println("workspace:", valueOr(s.WorktreePath, s.RepoPath))
	fmt.Println("brief:", valueOr(s.Brief.SourcePath, home.BriefPath(s.ID)))
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

func parseProfile(value string) types.AgentProfile {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "plan", "scope", "scoping":
		return types.ProfilePlan
	case "implement", "implementation":
		return types.ProfileImplement
	case "debug":
		return types.ProfileDebug
	case "review", "fix", "review-fix":
		return types.ProfileReview
	case "custom":
		return types.ProfileCustom
	default:
		return types.ProfileGeneral
	}
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
