package tmux

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/theforager/cmux/internal/process"
)

const prefix = "cmux"
const sep = "@"

type Session struct {
	Name     string
	Created  int64
	Title    string
	Dir      string
	Agent    string
	Kind     string
	ParentID string
}

type PaneInfo struct {
	Alive            bool
	PaneDead         bool
	ExitStatus       string
	LastActivityUnix int64
	CurrentCommand   string
}

type CreateOptions struct {
	Name     string
	Dir      string
	Command  string
	Title    string
	Agent    string
	Kind     string
	ParentID string
	Mobile   bool
}

func Exists() bool { return process.Exists("tmux") }

func Inside() bool { return os.Getenv("TMUX") != "" }

func List() ([]Session, error) {
	out, err := process.Run("tmux", "list-sessions", "-F", "#{session_name}|#{session_created}")
	if err != nil {
		if strings.Contains(err.Error(), "no server running") {
			return nil, nil
		}
		return nil, err
	}
	var sessions []Session
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" || !strings.HasPrefix(line, prefix+sep) {
			continue
		}
		parts := strings.Split(line, "|")
		created := parseInt64(parts[1])
		s := Session{Name: parts[0], Created: created}
		s.Title, _ = ShowEnv(s.Name, "CMUX_TITLE")
		s.Dir, _ = ShowEnv(s.Name, "CMUX_DIR")
		s.Agent, _ = ShowEnv(s.Name, "CMUX_AGENT")
		s.Kind, _ = ShowEnv(s.Name, "CMUX_KIND")
		s.ParentID, _ = ShowEnv(s.Name, "CMUX_PARENT_ID")
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func GenerateSessionName(dir, preferred string) (string, error) {
	base := ""
	if preferred != "" {
		base = prefix + sep + "agent" + sep + strings.ReplaceAll(preferred, sep, "-")
	} else {
		abs, err := filepathAbs(dir)
		if err != nil {
			return "", err
		}
		parent := baseName(dirName(abs))
		child := baseName(abs)
		base = prefix + sep + parent + sep + child
	}
	name := base
	for i := 2; ; i++ {
		if !Has(name) {
			return name, nil
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
}

func Has(name string) bool {
	_, err := process.Run("tmux", "has-session", "-t", name)
	return err == nil
}

func Create(o CreateOptions) error {
	_ = EnsureOptions()
	args := []string{"new-session", "-d", "-s", o.Name, "-c", o.Dir}
	if o.Mobile {
		args = append(args, "-x", "78")
	}
	if _, err := process.Run("tmux", args...); err != nil {
		return err
	}
	_ = SetEnv(o.Name, "CMUX_SESSION", o.Name)
	_ = SetEnv(o.Name, "CMUX_DIR", o.Dir)
	if o.Agent != "" {
		_ = SetEnv(o.Name, "CMUX_AGENT", o.Agent)
	}
	if o.Kind != "" {
		_ = SetEnv(o.Name, "CMUX_KIND", o.Kind)
	}
	if o.ParentID != "" {
		_ = SetEnv(o.Name, "CMUX_PARENT_ID", o.ParentID)
	}
	if o.Title != "" {
		_ = SetEnv(o.Name, "CMUX_TITLE", o.Title)
	}
	if o.Command != "" {
		_, err := process.Run("tmux", "send-keys", "-t", o.Name, "exec "+o.Command, "Enter")
		return err
	}
	return nil
}

func EnsureKeyOptions() error {
	return EnsureOptions()
}

func EnsureOptions() error {
	if err := EnsurePopupBinding(); err != nil {
		return err
	}
	// Preserve modern TUI styling such as subtle input-box backgrounds.
	_, _ = process.Run("tmux", "set-option", "-g", "default-terminal", "tmux-256color")
	ensureTerminalFeature("xterm*:RGB")
	ensureTerminalFeature("tmux*:RGB")
	ensureTerminalFeature("screen*:RGB")
	// Forward modified keys such as Option/Meta+Enter through tmux to TUIs.
	_, _ = process.Run("tmux", "set-option", "-s", "extended-keys", "always")
	_, _ = process.Run("tmux", "set-option", "-g", "xterm-keys", "on")
	_, _ = process.Run("tmux", "unbind-key", "-n", "M-Enter")
	ensureTerminalFeature("xterm*:extkeys")
	ensureTerminalFeature("tmux*:extkeys")
	ensureTerminalFeature("screen*:extkeys")
	return nil
}

func ensureTerminalFeature(entry string) {
	out, err := process.Run("tmux", "show-options", "-gqv", "terminal-features")
	if err == nil && strings.Contains(out, entry) {
		return
	}
	_, _ = process.Run("tmux", "set-option", "-as", "terminal-features", ","+entry)
}

func EnsurePopupBinding() error {
	_, err := process.Run("tmux", "bind-key", "g", "display-popup", "-E", "-w", "85%", "-h", "85%", "cmux switch")
	return err
}

func AttachOrSwitch(name string) error {
	_ = EnsureKeyOptions()
	if Inside() {
		_, err := process.Run("tmux", "switch-client", "-t", name)
		return err
	}
	path, err := exec.LookPath("tmux")
	if err != nil {
		return err
	}
	return syscall.Exec(path, []string{"tmux", "attach-session", "-t", name}, os.Environ())
}

func Find(target string) (Session, error) {
	sessions, err := List()
	if err != nil {
		return Session{}, err
	}
	for _, s := range sessions {
		if strings.HasSuffix(s.Name, sep+target) {
			return s, nil
		}
	}
	for _, s := range sessions {
		if strings.Contains(s.Name, target) {
			return s, nil
		}
	}
	return Session{}, fmt.Errorf("no session matching: %s", target)
}

func Kill(name string) error {
	_, err := process.Run("tmux", "kill-session", "-t", name)
	return err
}

func KillIfExists(name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	if err := Kill(name); err != nil && !IsMissingSessionError(err) {
		return err
	}
	return nil
}

func IsMissingSessionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "can't find session") || strings.Contains(msg, "no server running")
}

func Current() (string, error) {
	out, err := process.Run("tmux", "display-message", "-p", "#{session_name}")
	return strings.TrimSpace(out), err
}

func SetEnv(session, key, value string) error {
	_, err := process.Run("tmux", "set-environment", "-t", session, key, value)
	return err
}

func ShowEnv(session, key string) (string, error) {
	out, err := process.Run("tmux", "show-environment", "-t", session, key)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(out)
	prefix := key + "="
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("env not found")
	}
	return strings.TrimPrefix(line, prefix), nil
}

func Rename(current, child string) (string, error) {
	next := prefix + sep + Parent(current) + sep + strings.ReplaceAll(child, sep, "-")
	if _, err := process.Run("tmux", "rename-session", "-t", current, next); err != nil {
		return "", err
	}
	_ = SetEnv(next, "CMUX_SESSION", next)
	return next, nil
}

func SetTitle(session, title string) error {
	return SetEnv(session, "CMUX_TITLE", title)
}

func Capture(session string, lines int) string {
	out, err := process.Run("tmux", "capture-pane", "-t", session, "-p", "-S", fmt.Sprintf("-%d", lines*4))
	if err != nil {
		return ""
	}
	rows := strings.Split(out, "\n")
	var cleaned []string
	for _, row := range rows {
		row = strings.TrimRight(row, " \t\r")
		if strings.TrimSpace(row) == "" || strings.Contains(row, "for shortcuts") || strings.Contains(row, "for help") {
			continue
		}
		cleaned = append(cleaned, row)
	}
	if len(cleaned) > lines {
		cleaned = cleaned[len(cleaned)-lines:]
	}
	return strings.Join(cleaned, "\n")
}

func Inspect(session string) (PaneInfo, error) {
	out, err := process.Run("tmux", "display-message", "-t", session, "-p", "#{pane_last_activity}|#{pane_dead}|#{pane_dead_status}|#{pane_current_command}")
	if err != nil {
		if IsMissingSessionError(err) || strings.Contains(strings.ToLower(err.Error()), "no server running") {
			return PaneInfo{Alive: false}, nil
		}
		return PaneInfo{Alive: false}, err
	}
	parts := strings.Split(strings.TrimSpace(out), "|")
	info := PaneInfo{Alive: true}
	if len(parts) > 0 {
		info.LastActivityUnix = parseInt64(parts[0])
	}
	if len(parts) > 1 {
		info.PaneDead = parts[1] == "1"
	}
	if len(parts) > 2 {
		info.ExitStatus = strings.TrimSpace(parts[2])
	}
	if len(parts) > 3 {
		info.CurrentCommand = strings.TrimSpace(parts[3])
	}
	return info, nil
}

func InferStatus(session string) string {
	info, err := Inspect(session)
	if err != nil {
		return "crashed"
	}
	if !info.Alive || info.PaneDead {
		return "crashed"
	}
	idle := time.Now().Unix() - info.LastActivityUnix
	preview := Capture(session, 5)
	lastLine := ""
	lines := strings.Split(preview, "\n")
	if len(lines) > 0 {
		lastLine = strings.TrimSpace(lines[len(lines)-1])
	}
	lower := strings.ToLower(lastLine)
	if strings.Contains(lower, "error") || strings.Contains(lower, "failed") || strings.Contains(lower, "fatal") {
		return "crashed"
	}
	if idle < 2 {
		return "running"
	}
	if strings.HasPrefix(lastLine, ">") || strings.HasSuffix(lastLine, "$") || strings.HasSuffix(lastLine, "%") || strings.HasSuffix(lastLine, "❯") {
		return "waiting_for_input"
	}
	return "idle"
}

func Debug(w io.Writer) error {
	fmt.Fprintln(w, "tmux sessions:")
	if out, err := process.Run("tmux", "list-sessions", "-F", "#{session_name}|attached=#{session_attached}|windows=#{session_windows}"); err == nil {
		fmt.Fprintln(w, strings.TrimSpace(out))
	} else {
		fmt.Fprintln(w, err)
	}
	fmt.Fprintln(w, "\ntmux panes:")
	if out, err := process.Run("tmux", "list-panes", "-a", "-F", "#{session_name}|#{window_index}.#{pane_index}|cmd=#{pane_current_command}|dead=#{pane_dead}|status=#{pane_dead_status}|path=#{pane_current_path}"); err == nil {
		fmt.Fprintln(w, strings.TrimSpace(out))
	} else {
		fmt.Fprintln(w, err)
	}
	return nil
}

func Parent(name string) string {
	parts := strings.Split(name, sep)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

func Child(name string) string {
	parts := strings.Split(name, sep)
	if len(parts) > 2 {
		return strings.Join(parts[2:], sep)
	}
	return name
}

func parseInt64(v string) int64 {
	var n int64
	for _, r := range strings.TrimSpace(v) {
		if r >= '0' && r <= '9' {
			n = n*10 + int64(r-'0')
		}
	}
	return n
}

func filepathAbs(path string) (string, error) {
	return filepath.Abs(path)
}

func baseName(path string) string {
	return filepath.Base(path)
}

func dirName(path string) string {
	return filepath.Dir(path)
}
