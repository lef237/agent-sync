package discovery

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lef237/agent-sync/internal/model"
)

// skippedAnywhere never holds hand-written instructions, at any depth.
var skippedAnywhere = map[string]bool{
	"node_modules": true,
}

// skippedAtRoot are conventional build output names. They are only skipped at
// the repository root, because nested directories with these names are
// usually real source directories -- this repository has internal/target, and
// packages/build or packages/dist are just as ordinary. Skipping them at
// every depth silently dropped their AGENTS.md with no way to notice.
var skippedAtRoot = map[string]bool{
	"build":  true,
	"dist":   true,
	"out":    true,
	"target": true,
	"vendor": true,
}

func RepoRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		fi, err := os.Stat(filepath.Join(dir, ".git"))
		if err == nil && (fi.IsDir() || fi.Mode().IsRegular()) {
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

	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// A directory agent-sync cannot read is worth reporting, but it
			// must not take the whole run down: one unreadable directory
			// anywhere in the tree used to abort discovery entirely.
			if d != nil && d.IsDir() {
				st.DiscoveryIncomplete = true
				st.Warnings = append(st.Warnings, fmt.Sprintf("skipping %s: %v", displayPath(root, path), err))
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if !skipDir(d.Name(), rel) {
				return nil
			}
			if w := nestedSkillsWarning(path, rel); w != "" {
				st.Warnings = append(st.Warnings, w)
			}
			return filepath.SkipDir
		}
		if d.Name() != "AGENTS.md" && d.Name() != "AGENTS.override.md" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		found = append(found, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}

	found = withoutIgnored(root, found)
	for _, rel := range found {
		st.Instructions = append(st.Instructions, model.Instruction{
			Dir:    filepath.Dir(rel),
			Source: rel,
		})
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

func skipDir(name, rel string) bool {
	// Dot directories are tooling and environment state: .git, .claude,
	// .agents, .venv, .tox, .next and the rest. None of them hold
	// instructions meant for a coding agent to load.
	if strings.HasPrefix(name, ".") {
		return true
	}
	if skippedAnywhere[name] {
		return true
	}
	return skippedAtRoot[name] && filepath.Dir(rel) == "."
}

// nestedSkillsWarning reports skills placed outside the repository root, which
// agent-sync does not sync: Claude Code only reads .claude/skills at the
// project root, so mirroring them per directory would produce links nothing
// loads.
func nestedSkillsWarning(path, rel string) string {
	if filepath.Base(path) != ".agents" || filepath.Dir(rel) == "." {
		return ""
	}
	if fi, err := os.Stat(filepath.Join(path, "skills")); err != nil || !fi.IsDir() {
		return ""
	}
	return fmt.Sprintf("%s/skills is not synced; skills are only read from the repository root", filepath.ToSlash(rel))
}

func displayPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// withoutIgnored drops paths git ignores, so agent-sync does not write a
// CLAUDE.md into a virtualenv, a cache, or any other directory the project
// has already excluded. Tracked files stay even when a rule matches them, and
// when git cannot answer every candidate is kept.
func withoutIgnored(root string, paths []string) []string {
	if len(paths) == 0 {
		return paths
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return paths
	}

	ignored := gitPaths(root, paths, "check-ignore", "--stdin", "-z")
	if len(ignored) == 0 {
		return paths
	}
	for tracked := range gitPaths(root, nil, append([]string{"ls-files", "-z", "--"}, ignored.keys()...)...) {
		delete(ignored, tracked)
	}
	if len(ignored) == 0 {
		return paths
	}

	kept := make([]string, 0, len(paths))
	for _, p := range paths {
		if !ignored[p] {
			kept = append(kept, p)
		}
	}
	return kept
}

// gitPaths runs one git command that emits NUL-separated paths. stdin, when
// given, is fed to the command the same way. A failure other than "nothing
// matched" yields no paths, which always means "keep the candidate".
func gitPaths(root string, stdin []string, args ...string) pathSet {
	git, err := exec.LookPath("git")
	if err != nil {
		return nil
	}
	cmd := exec.Command(git, args...)
	cmd.Dir = root
	if stdin != nil {
		cmd.Stdin = strings.NewReader(strings.Join(stdin, "\x00") + "\x00")
	}

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return nil
		}
	}

	set := pathSet{}
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			set[filepath.ToSlash(p)] = true
		}
	}
	return set
}

type pathSet map[string]bool

func (s pathSet) keys() []string {
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
