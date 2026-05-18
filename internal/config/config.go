package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/theforager/cmux/internal/home"
	"github.com/theforager/cmux/internal/types"
)

func Default() types.Config {
	return types.Config{
		DefaultQueuePreset: "Agent Ready",
		QueuePresets: []types.QueuePreset{
			{Name: "Agent Ready", AssigneeMode: "unassigned", Limit: 8},
			{Name: "My Active", AssigneeMode: "viewer", Limit: 8},
			{Name: "Needs Review", Limit: 8},
		},
	}
}

func Load() (types.Config, error) {
	var cfg types.Config
	b, err := os.ReadFile(home.ConfigPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return types.Config{}, nil
		}
		return types.Config{}, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return types.Config{}, fmt.Errorf("read %s: %w", home.ConfigPath(), err)
	}
	return cfg, nil
}

func LoadOrDefault() (types.Config, error) {
	cfg, err := Load()
	if err != nil {
		return types.Config{}, err
	}
	if len(cfg.QueuePresets) == 0 {
		def := Default()
		cfg.QueuePresets = def.QueuePresets
	}
	if cfg.DefaultQueuePreset == "" && len(cfg.QueuePresets) > 0 {
		cfg.DefaultQueuePreset = cfg.QueuePresets[0].Name
	}
	return cfg, nil
}

func Save(cfg types.Config) error {
	cfg = normalizeRepos(cfg)
	cfg = normalizeEditors(cfg)
	if err := os.MkdirAll(home.Dir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(home.ConfigPath(), b, 0o644)
}

func AddRepo(cfg types.Config, repo types.RepoConfig) types.Config {
	repo.Name = strings.TrimSpace(repo.Name)
	repo.Path = strings.TrimSpace(repo.Path)
	if repo.Path == "" {
		return cfg
	}
	if repo.Name == "" {
		repo.Name = filepath.Base(repo.Path)
	}
	cfg.Repos = append(cfg.Repos, repo)
	return normalizeRepos(cfg)
}

func RememberRepo(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	cfg, err := Load()
	if err != nil {
		return err
	}
	if cfg.DefaultRepoPath == "" {
		cfg.DefaultRepoPath = path
	}
	cfg = AddRepo(cfg, types.RepoConfig{Name: filepath.Base(path), Path: path})
	return Save(cfg)
}

func RememberEditor(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	cfg, err := Load()
	if err != nil {
		return err
	}
	cfg.DefaultEditorCommand = command
	cfg.EditorCommands = append(cfg.EditorCommands, command)
	return Save(cfg)
}

func RememberSSHTarget(target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	cfg, err := Load()
	if err != nil {
		return err
	}
	cfg.DefaultSSHTarget = target
	return Save(cfg)
}

func normalizeRepos(cfg types.Config) types.Config {
	seen := map[string]bool{}
	repos := []types.RepoConfig{}
	for _, repo := range cfg.Repos {
		repo.Name = strings.TrimSpace(repo.Name)
		repo.Path = strings.TrimSpace(repo.Path)
		if repo.Path == "" || seen[repo.Path] {
			continue
		}
		if repo.Name == "" {
			repo.Name = filepath.Base(repo.Path)
		}
		seen[repo.Path] = true
		repos = append(repos, repo)
	}
	cfg.Repos = repos
	return cfg
}

func normalizeEditors(cfg types.Config) types.Config {
	seen := map[string]bool{}
	commands := []string{}
	cfg.DefaultEditorCommand = strings.TrimSpace(cfg.DefaultEditorCommand)
	add := func(command string) {
		command = strings.TrimSpace(command)
		if command == "" || seen[command] {
			return
		}
		seen[command] = true
		commands = append(commands, command)
	}
	add(cfg.DefaultEditorCommand)
	for _, command := range cfg.EditorCommands {
		add(command)
	}
	cfg.EditorCommands = commands
	return cfg
}

func Preset(cfg types.Config, name string) (types.QueuePreset, bool) {
	if name == "" {
		name = cfg.DefaultQueuePreset
	}
	if name == "" && len(cfg.QueuePresets) > 0 {
		return cfg.QueuePresets[0], true
	}
	for _, preset := range cfg.QueuePresets {
		if preset.Name == name {
			if preset.RepoPath == "" {
				preset.RepoPath = cfg.DefaultRepoPath
			}
			return preset, true
		}
	}
	return types.QueuePreset{}, false
}
