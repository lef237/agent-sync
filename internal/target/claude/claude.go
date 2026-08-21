package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"agent-sync/internal/apply"
	"agent-sync/internal/model"
	"agent-sync/internal/planner"
	"agent-sync/internal/target"
)

type Claude struct{}

func New() *Claude { return &Claude{} }

func (t *Claude) Name() string { return "claude" }

func (t *Claude) Plan(root string, src *model.SourceState) (*planner.Plan, error) {
	p := &planner.Plan{}
	if err := t.planInstructions(root, src, p); err != nil {
		return nil, err
	}
	if err := t.planSkills(root, src, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (t *Claude) planInstructions(root string, src *model.SourceState, p *planner.Plan) error {
	byDir := map[string][]model.Instruction{}
	for _, in := range src.Instructions {
		byDir[in.Dir] = append(byDir[in.Dir], in)
	}
	for _, dir := range sortedKeys(byDir) {
		importTarget := "AGENTS.md"
		for _, in := range byDir[dir] {
			if strings.HasSuffix(in.Source, "AGENTS.override.md") {
				importTarget = "AGENTS.override.md"
			}
		}
		block := planner.StartMarker + "\n@" + importTarget + "\n" + planner.EndMarker
		relPath := filepath.ToSlash(filepath.Join(dir, "CLAUDE.md"))
		existing, err := os.ReadFile(filepath.Join(root, relPath))
		if os.IsNotExist(err) {
			p.Add(planner.Create{Path: relPath, Content: block + "\n"})
			continue
		}
		if err != nil {
			return err
		}
		next, changed := planner.ApplyManagedBlock(string(existing), block)
		if changed {
			p.Add(planner.Update{Path: relPath, Content: next})
		}
	}
	return nil
}

func (t *Claude) planSkills(root string, src *model.SourceState, p *planner.Plan) error {
	st := apply.LoadState(root)
	desired := map[string]bool{}
	for _, sk := range src.Skills {
		desired[sk.Name] = true
		dst := filepath.Join(root, sk.Dst)
		srcAbs := filepath.Join(root, sk.Src)
		rel, err := filepath.Rel(filepath.Dir(dst), srcAbs)
		if err != nil {
			return err
		}
		fi, err := os.Lstat(dst)
		if os.IsNotExist(err) {
			p.Add(planner.CreateLink{Path: sk.Dst, Target: rel})
			continue
		}
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			if contains(st.ManagedSymlinks, sk.Name) {
				return fmt.Errorf("%s is managed by agent-sync but is no longer a symlink; remove it manually", dst)
			}
			continue
		}
		cur, err := os.Readlink(dst)
		if err != nil {
			return err
		}
		if cur != rel {
			p.Add(planner.RemoveLink{Path: sk.Dst})
			p.Add(planner.CreateLink{Path: sk.Dst, Target: rel})
		}
	}

	for _, name := range st.ManagedSymlinks {
		if desired[name] {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join(".claude", "skills", name))
		dst := filepath.Join(root, relPath)
		if fi, err := os.Lstat(dst); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			p.Add(planner.RemoveLink{Path: relPath})
		}
	}
	return nil
}

func sortedKeys(m map[string][]model.Instruction) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

var _ target.Target = (*Claude)(nil)