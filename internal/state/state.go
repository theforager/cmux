package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/theforager/cmux/internal/format"
	"github.com/theforager/cmux/internal/types"
)

func Dir() string {
	if v := os.Getenv("CMUX_STATE_DIR"); v != "" {
		return v
	}
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, "cmux")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "cmux")
}

func agentsDir() string {
	return filepath.Join(Dir(), "agents")
}

func ensure() error {
	return os.MkdirAll(agentsDir(), 0o755)
}

func path(id string) string {
	return filepath.Join(agentsDir(), id+".json")
}

func Read(id string) (types.AgentSession, error) {
	var s types.AgentSession
	b, err := os.ReadFile(path(id))
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(b, &s)
	return s, err
}

func Write(s types.AgentSession) error {
	if err := ensure(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path(s.ID), b, 0o644)
}

func List() ([]types.AgentSession, error) {
	if err := ensure(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(agentsDir())
	if err != nil {
		return nil, err
	}
	var out []types.AgentSession
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(agentsDir(), entry.Name()))
		if err != nil {
			continue
		}
		var s types.AgentSession
		if json.Unmarshal(b, &s) == nil {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastUpdatedAt > out[j].LastUpdatedAt
	})
	return out, nil
}

func Update(id string, fn func(types.AgentSession) types.AgentSession) (types.AgentSession, error) {
	s, err := Read(id)
	if err != nil {
		return s, err
	}
	next := fn(s)
	next.LastUpdatedAt = format.Now()
	return next, Write(next)
}

func NextScratchID() (string, error) {
	sessions, err := List()
	if err != nil {
		return "", err
	}
	seen := map[string]bool{}
	for _, s := range sessions {
		seen[s.ID] = true
	}
	stamp := time.Now().Format("20060102")
	for i := 1; i < 10000; i++ {
		id := "scratch-" + stamp + "-" + itoa(i)
		if !seen[id] {
			return id, nil
		}
	}
	return "", errors.New("could not allocate scratch id")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := []byte{}
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}
