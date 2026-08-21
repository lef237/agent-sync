package discovery

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/lef237/agent-sync/internal/model"
)

var skipped = map[string]bool{
	".git":         true,
	".hg":          true,
	".svn":         true,
	"node_modules": true,
	"vendor":       true,
	".agents":      true,
	".claude":      true,
	".cursor":      true,
	".idea":        true,
	".vscode":      true,
	"build":        true,
	"dist":         true,
	"out":          true,
	"target":       true,
}

func RepoRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		fi, err := os.Stat(filepath.Join(dir, ".git"))
		if err == nil && fi.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

func Discover(root string) (*model.SourceState, error) {
	st := &model.SourceState{Root: root}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipped[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "AGENTS.md" && d.Name() != "AGENTS.override.md" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		st.Instructions = append(st.Instructions, model.Instruction{
			Dir:    filepath.Dir(rel),
			Source: rel,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	skillsRoot := filepath.Join(root, ".agents", "skills")
	entries, err := os.ReadDir(skillsRoot)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			st.Skills = append(st.Skills, model.Skill{
				Name: e.Name(),
				Src:  filepath.ToSlash(filepath.Join(".agents", "skills", e.Name())),
				Dst:  filepath.ToSlash(filepath.Join(".claude", "skills", e.Name())),
			})
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	sort.Slice(st.Instructions, func(i, j int) bool {
		if st.Instructions[i].Dir != st.Instructions[j].Dir {
			return st.Instructions[i].Dir < st.Instructions[j].Dir
		}
		return st.Instructions[i].Source < st.Instructions[j].Source
	})
	sort.Slice(st.Skills, func(i, j int) bool {
		return st.Skills[i].Name < st.Skills[j].Name
	})
	return st, nil
}