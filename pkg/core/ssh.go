package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/theforager/cmux/internal/process"
)

type SSHHost struct {
	Alias  string   `json:"alias,omitempty"`
	Target string   `json:"target"`
	Args   []string `json:"args,omitempty"`
	Source string   `json:"source,omitempty"`
}

type RemoteCommandResult struct {
	Host   SSHHost  `json:"host"`
	Args   []string `json:"args,omitempty"`
	Output string   `json:"output,omitempty"`
}

func ListSSHHosts() ([]SSHHost, error) {
	path := sshHostsPath()
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return ParseSSHHosts(string(b), path), nil
}

func ParseSSHHosts(contents, source string) []SSHHost {
	var hosts []SSHHost
	for _, line := range strings.Split(contents, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		alias, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		alias = strings.TrimSpace(alias)
		args := strings.Fields(strings.TrimSpace(value))
		if alias == "" || len(args) == 0 {
			continue
		}
		hosts = append(hosts, SSHHost{Alias: alias, Target: args[len(args)-1], Args: args, Source: source})
	}
	return hosts
}

func ResolveSSHHost(target string) (SSHHost, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return SSHHost{}, fmt.Errorf("ssh host is required")
	}
	hosts, err := ListSSHHosts()
	if err != nil {
		return SSHHost{}, err
	}
	for _, host := range hosts {
		if host.Alias == target {
			return host, nil
		}
	}
	return SSHHost{Target: target, Args: []string{target}}, nil
}

func RemoteCommand(target string, cmuxArgs ...string) (RemoteCommandResult, error) {
	host, err := ResolveSSHHost(target)
	if err != nil {
		return RemoteCommandResult{}, err
	}
	args := append([]string{"-T"}, host.Args...)
	args = append(args, append([]string{"cmux"}, cmuxArgs...)...)
	output, err := process.Run("ssh", args...)
	return RemoteCommandResult{Host: host, Args: cmuxArgs, Output: strings.TrimSpace(output)}, err
}

func RemoteDoctor(target string) (RemoteCommandResult, error) {
	return RemoteCommand(target, "doctor")
}

func RemoteInventory(target string, scan bool) (RemoteCommandResult, error) {
	args := []string{"inventory"}
	if scan {
		args = append(args, "--scan")
	}
	return RemoteCommand(target, args...)
}

func sshHostsPath() string {
	return filepath.Join(configHome(), "cmux", "hosts")
}

func configHome() string {
	if value := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); value != "" {
		return value
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		return filepath.Join(".", ".config")
	}
	return filepath.Join(home, ".config")
}
