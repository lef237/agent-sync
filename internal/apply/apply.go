package apply

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/lef237/agent-sync/internal/model"
	"github.com/lef237/agent-sync/internal/planner"
)

const stateFileName = ".agent-sync.json"

func StatePath(root string) string {
	return filepath.Join(root, ".claude", stateFileName)
}

func LoadState(root string) *model.State {
	st := &model.State{Version: 1}
	b, err := os.ReadFile(StatePath(root))
	if err != nil {
		return st
	}
	if err := json.Unmarshal(b, st); err != nil {
		return &model.State{Version: 1}
	}
	return st
}

func SaveState(root string, st *model.State) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(StatePath(root)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(StatePath(root), append(b, '\n'), 0o644)
}

func Apply(root string, plan *planner.Plan, st *model.State) error {
	managed := map[string]bool{}
	for _, n := range st.ManagedSymlinks {
		managed[n] = true
	}
	for _, a := range plan.Actions {
		switch act := a.(type) {
		case planner.Create:
			if err := writeFile(root, act.Path, act.Content); err != nil {
				return err
			}
		case planner.Update:
			if err := writeFile(root, act.Path, act.Content); err != nil {
				return err
			}
		case planner.CreateLink:
			dst := filepath.Join(root, act.Path)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(act.Target, dst); err != nil {
				fi, lerr := os.Lstat(dst)
				if lerr != nil {
					return err
				}
				if fi.IsDir() {
					return fmt.Errorf("%s exists as a directory; remove it manually", dst)
				}
				if err := os.Remove(dst); err != nil {
					return err
				}
				if err := os.Symlink(act.Target, dst); err != nil {
					return err
				}
			}
			managed[filepath.Base(act.Path)] = true
		case planner.RemoveLink:
			if err := os.Remove(filepath.Join(root, act.Path)); err != nil && !os.IsNotExist(err) {
				return err
			}
			delete(managed, filepath.Base(act.Path))
		}
	}
	st.ManagedSymlinks = st.ManagedSymlinks[:0]
	for n := range managed {
		st.ManagedSymlinks = append(st.ManagedSymlinks, n)
	}
	sort.Strings(st.ManagedSymlinks)
	return SaveState(root, st)
}

func writeFile(root, path, content string) error {
	abs := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(content), 0o644)
}