package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lef237/agent-sync/internal/apply"
	"github.com/lef237/agent-sync/internal/model"
	"github.com/lef237/agent-sync/internal/planner"
	"github.com/lef237/agent-sync/internal/target"
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
		if err := apply.ValidateOutputPath(root, relPath); err != nil {
			return err
		}
		existing, err := os.ReadFile(filepath.Join(root, relPath))
		if os.IsNotExist(err) {
			p.Add(planner.Create{Path: relPath, Content: block + "\n"})
			continue
		}
		if err != nil {
			return err
		}
		next, changed, err := planner.ApplyManagedBlock(string(existing), block)
		if err != nil {
			return fmt.Errorf("%s: %w", relPath, err)
		}
		if changed {
			p.Add(planner.Update{Path: relPath, Content: next})
		}
	}
	return nil
}

func (t *Claude) planSkills(root string, src *model.SourceState, p *planner.Plan) error {
	st, err := apply.LoadState(root)
	if err != nil {
		return err
	}
	owned := st.Target(t.Name())
	managed := make(map[string]model.ManagedSymlink, len(owned.ManagedSymlinks))
	for _, link := range owned.ManagedSymlinks {
		managed[link.Name] = link
	}
	desired := map[string]bool{}
	for _, sk := range src.Skills {
		desired[sk.Name] = true
		if err := apply.ValidateLinkPath(root, sk.Dst); err != nil {
			return err
		}
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
			if _, ok := managed[sk.Name]; ok {
				return fmt.Errorf("%s is managed by agent-sync but is no longer a symlink; remove it manually", dst)
			}
			continue
		}
		cur, err := os.Readlink(dst)
		if err != nil {
			return err
		}

		link, isManaged := managed[sk.Name]
		if !isManaged {
			// A symlink that is not recorded as ours belongs to the user,
			// even when it has the same skill name.
			continue
		}
		if link.Target == "" {
			// Legacy state recorded only names. Re-adopt an existing link only
			// when its target exactly matches the deterministic target we would
			// create; otherwise preserve it as a user-owned link.
			if cur == rel {
				p.Add(planner.AdoptLink{Path: sk.Dst, Target: rel})
			} else {
				p.Add(planner.ForgetLink{Path: sk.Dst})
			}
			continue
		}
		if cur != link.Target {
			return fmt.Errorf("%s was changed outside agent-sync; remove it manually", dst)
		}
		if cur != rel {
			p.Add(planner.RemoveLink{Path: sk.Dst, Target: cur})
			p.Add(planner.CreateLink{Path: sk.Dst, Target: rel})
		}
	}

	for _, link := range owned.ManagedSymlinks {
		if desired[link.Name] {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join(".claude", "skills", link.Name))
		if err := apply.ValidateLinkPath(root, relPath); err != nil {
			return err
		}
		dst := filepath.Join(root, relPath)
		fi, err := os.Lstat(dst)
		if os.IsNotExist(err) {
			p.Add(planner.ForgetLink{Path: relPath})
			continue
		}
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			p.Add(planner.ForgetLink{Path: relPath})
			continue
		}
		cur, err := os.Readlink(dst)
		if err != nil {
			return err
		}
		if link.Target != "" && cur == link.Target {
			p.Add(planner.RemoveLink{Path: relPath, Target: cur})
		} else {
			// The link was replaced or came from legacy state. Preserve it.
			p.Add(planner.ForgetLink{Path: relPath})
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

var _ target.Target = (*Claude)(nil)
