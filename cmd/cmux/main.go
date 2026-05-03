package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/theforager/cmux/internal/agent"
	"github.com/theforager/cmux/internal/format"
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
	cmd.AddCommand(newCmd(), listCmd(), attachCmd(), switchCmd(), killCmd(), titleCmd(), infoCmd(), debugCmd(), agentCmd(), doctorCmd(), legacyCmd())
	return cmd
}

func legacyCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "legacy [args...]",
		Short:              "Run the preserved Bash implementation",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			repo := filepath.Dir(filepath.Dir(exe))
			if filepath.Base(filepath.Dir(exe)) != "dist" {
				repo, _ = os.Getwd()
			}
			script := filepath.Join(repo, "legacy", "cmux.bash")
			c := exec.Command(script, args...)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
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
			fmt.Println("state:", state.Dir())
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
	cmd.AddCommand(agentStartCmd(), agentScratchCmd(), agentListCmd(), agentOpenCmd(), agentStatusCmd("status"), agentStatusCmd("block"), agentStatusCmd("review"), agentStatusCmd("done"))
	return cmd
}

func agentStartCmd() *cobra.Command {
	var title, agentCommand string
	var worktree, noWorktree, prepare bool
	cmd := &cobra.Command{
		Use:   "start [ISSUE]",
		Short: "Start a Linear issue-backed or task-backed agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			var issue string
			if len(args) > 0 {
				issue = args[0]
			}
			s, err := agent.Start(agent.StartOptions{Cwd: ".", IssueKey: issue, Title: title, Agent: agentCommand, Worktree: worktree, NoWorktree: noWorktree, PrepareOnly: prepare})
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
			s, err := agent.SetStatus(id, status, summary)
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
