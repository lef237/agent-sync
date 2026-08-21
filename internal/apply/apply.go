package apply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lef237/agent-sync/internal/model"
	"github.com/lef237/agent-sync/internal/planner"
)

const stateFileName = ".agent-sync.json"

func StatePath(root string) string {
	return filepath.Join(root, ".claude", stateFileName)
}

// LoadState returns an empty state only when the state file does not exist.
// Permission errors and malformed state must be visible to the caller;
// otherwise a sync could silently lose ownership information.
func LoadState(root string) (*model.State, error) {
	relPath := filepath.ToSlash(filepath.Join(".claude", stateFileName))
	if err := ValidateOutputPath(root, relPath); err != nil {
		return nil, err
	}

	b, err := os.ReadFile(StatePath(root))
	if os.IsNotExist(err) {
		return &model.State{Version: 1}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", StatePath(root), err)
	}

	st, err := decodeState(b)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", StatePath(root), err)
	}
	if st.Version == 0 {
		st.Version = 1
	}
	if st.Version != 1 {
		return nil, fmt.Errorf("unsupported state version %d in %s", st.Version, StatePath(root))
	}
	if err := validateState(st); err != nil {
		return nil, fmt.Errorf("invalid state %s: %w", StatePath(root), err)
	}
	return st, nil
}

// decodeState accepts both the current object form and the original string
// form of managedSymlinks so existing repositories can migrate safely.
func decodeState(b []byte) (*model.State, error) {
	var raw struct {
		Version         int             `json:"version"`
		ManagedSymlinks json.RawMessage `json:"managedSymlinks"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}

	st := &model.State{Version: raw.Version}
	data := bytes.TrimSpace(raw.ManagedSymlinks)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return st, nil
	}

	if err := json.Unmarshal(data, &st.ManagedSymlinks); err == nil {
		return st, nil
	}

	var legacy []string
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("managedSymlinks must be an array of managed links: %w", err)
	}
	st.ManagedSymlinks = nil
	for _, name := range legacy {
		st.ManagedSymlinks = append(st.ManagedSymlinks, model.ManagedSymlink{Name: name})
	}
	return st, nil
}

func validateState(st *model.State) error {
	seen := make(map[string]bool, len(st.ManagedSymlinks))
	for _, link := range st.ManagedSymlinks {
		if !validSkillName(link.Name) {
			return fmt.Errorf("invalid managed skill name %q", link.Name)
		}
		if seen[link.Name] {
			return fmt.Errorf("duplicate managed skill name %q", link.Name)
		}
		seen[link.Name] = true
	}
	return nil
}

func validSkillName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return filepath.Base(filepath.FromSlash(name)) == name
}

func SaveState(root string, st *model.State) error {
	if st == nil {
		return fmt.Errorf("cannot save a nil state")
	}
	if st.Version == 0 {
		st.Version = 1
	}
	if st.Version != 1 {
		return fmt.Errorf("unsupported state version %d", st.Version)
	}
	if err := validateState(st); err != nil {
		return err
	}

	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	relPath := filepath.ToSlash(filepath.Join(".claude", stateFileName))
	return writeAtomic(root, relPath, b, 0o644, false)
}

func Apply(root string, plan *planner.Plan, st *model.State) error {
	if st == nil {
		return fmt.Errorf("cannot apply a plan with nil state")
	}

	managed := make(map[string]string, len(st.ManagedSymlinks))
	for _, link := range st.ManagedSymlinks {
		managed[link.Name] = link.Target
	}

	for _, a := range plan.Actions {
		switch act := a.(type) {
		case planner.Create:
			if err := writeFile(root, act.Path, act.Content, true); err != nil {
				return err
			}
		case planner.Update:
			if err := writeFile(root, act.Path, act.Content, false); err != nil {
				return err
			}
		case planner.CreateLink:
			if err := createLink(root, act); err != nil {
				return err
			}
			managed[skillName(act.Path)] = act.Target
		case planner.AdoptLink:
			if err := verifyLink(root, act.Path, act.Target); err != nil {
				return err
			}
			managed[skillName(act.Path)] = act.Target
		case planner.RemoveLink:
			if err := removeLink(root, act); err != nil {
				return err
			}
			delete(managed, skillName(act.Path))
		case planner.ForgetLink:
			delete(managed, skillName(act.Path))
		default:
			return fmt.Errorf("unsupported action %T", a)
		}
	}

	st.ManagedSymlinks = make([]model.ManagedSymlink, 0, len(managed))
	for name, target := range managed {
		st.ManagedSymlinks = append(st.ManagedSymlinks, model.ManagedSymlink{
			Name:   name,
			Target: target,
		})
	}
	sort.Slice(st.ManagedSymlinks, func(i, j int) bool {
		return st.ManagedSymlinks[i].Name < st.ManagedSymlinks[j].Name
	})
	return SaveState(root, st)
}

func skillName(path string) string {
	return filepath.Base(filepath.FromSlash(filepath.Clean(path)))
}

func createLink(root string, act planner.CreateLink) error {
	if err := ValidateLinkPath(root, act.Path); err != nil {
		return err
	}
	if err := ensureParentDir(root, act.Path); err != nil {
		return err
	}
	_, abs, _, err := safeAbs(root, act.Path)
	if err != nil {
		return err
	}

	if fi, err := os.Lstat(abs); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			cur, readErr := os.Readlink(abs)
			if readErr == nil && cur == act.Target {
				return nil
			}
		}
		return fmt.Errorf("%s appeared while applying; remove it manually", abs)
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.Symlink(act.Target, abs); err != nil {
		// A concurrent sync may have created the exact desired link. Accept
		// that harmless race, but never remove an unexpected path.
		if fi, lerr := os.Lstat(abs); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
			if cur, rerr := os.Readlink(abs); rerr == nil && cur == act.Target {
				return nil
			}
		}
		return err
	}
	return nil
}

func removeLink(root string, act planner.RemoveLink) error {
	if err := ValidateLinkPath(root, act.Path); err != nil {
		return err
	}
	_, abs, _, err := safeAbs(root, act.Path)
	if err != nil {
		return err
	}
	fi, err := os.Lstat(abs)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s is no longer a symlink; leave it untouched", abs)
	}
	cur, err := os.Readlink(abs)
	if err != nil {
		return err
	}
	if act.Target == "" || cur != act.Target {
		return fmt.Errorf("%s changed since planning; leave it untouched", abs)
	}
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func verifyLink(root, path, target string) error {
	if err := ValidateLinkPath(root, path); err != nil {
		return err
	}
	_, abs, _, err := safeAbs(root, path)
	if err != nil {
		return err
	}
	fi, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s is no longer a symlink; leave it untouched", abs)
	}
	cur, err := os.Readlink(abs)
	if err != nil {
		return err
	}
	if target == "" || cur != target {
		return fmt.Errorf("%s changed since planning; leave it untouched", abs)
	}
	return nil
}

func writeFile(root, path, content string, createOnly bool) error {
	if err := ValidateOutputPath(root, path); err != nil {
		return err
	}
	if err := ensureParentDir(root, path); err != nil {
		return err
	}
	_, abs, _, err := safeAbs(root, path)
	if err != nil {
		return err
	}

	perm := os.FileMode(0o644)
	if fi, err := os.Lstat(abs); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink; remove it manually", abs)
		}
		if fi.IsDir() {
			return fmt.Errorf("%s is a directory", abs)
		}
		perm = fi.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeAtomic(root, path, []byte(content), perm, createOnly)
}

func writeAtomic(root, path string, content []byte, perm os.FileMode, createOnly bool) error {
	if err := ValidateOutputPath(root, path); err != nil {
		return err
	}
	if err := ensureParentDir(root, path); err != nil {
		return err
	}
	_, abs, _, err := safeAbs(root, path)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(abs), ".agent-sync-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if createOnly {
		// Hard-linking the completed temporary file gives create semantics
		// without overwriting a path that appeared after planning.
		return os.Link(tmpName, abs)
	}
	if err := os.Rename(tmpName, abs); err == nil {
		return nil
	} else if !os.IsExist(err) {
		return err
	}

	// Windows does not replace an existing file with Rename. Preserve the
	// same safety rule there: only remove a regular file that is still at the
	// destination, never a symlink or another unexpected object.
	fi, lerr := os.Lstat(abs)
	if lerr != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 || fi.IsDir() {
		return fmt.Errorf("%s changed while applying", abs)
	}
	if lerr := os.Remove(abs); lerr != nil {
		return lerr
	}
	return os.Rename(tmpName, abs)
}

// ValidateOutputPath rejects symlinks in all existing path components,
// including the final file. It is used for files that agent-sync writes.
func ValidateOutputPath(root, rel string) error {
	return validatePath(root, rel, false)
}

// ValidateLinkPath rejects symlinks in parent directories but allows the
// final component to be a symlink so callers can inspect or unlink it safely.
func ValidateLinkPath(root, rel string) error {
	return validatePath(root, rel, true)
}

func validatePath(root, rel string, allowFinalSymlink bool) error {
	rootAbs, abs, clean, err := safeAbs(root, rel)
	if err != nil {
		return err
	}
	current := rootAbs
	parts := strings.Split(clean, string(filepath.Separator))
	for i, part := range parts {
		current = filepath.Join(current, part)
		fi, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		final := i == len(parts)-1
		if fi.Mode()&os.ModeSymlink != 0 && (!final || !allowFinalSymlink) {
			return fmt.Errorf("unsafe symlink path %s", abs)
		}
		if !final && !fi.IsDir() {
			return fmt.Errorf("path component %s is not a directory", current)
		}
	}
	return nil
}

func ensureParentDir(root, rel string) error {
	_, _, clean, err := safeAbs(root, rel)
	if err != nil {
		return err
	}
	parent := filepath.Dir(clean)
	if parent == "." {
		return nil
	}
	return ensureDir(root, parent)
}

func ensureDir(root, rel string) error {
	rootAbs, _, clean, err := safeAbs(root, rel)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(rootAbs); err != nil {
		return err
	} else if !fi.IsDir() {
		return fmt.Errorf("root %s is not a directory", rootAbs)
	}

	current := rootAbs
	if clean == "." {
		return nil
	}
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		fi, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err := os.Mkdir(current, 0o755); err != nil && !os.IsExist(err) {
				return err
			}
			fi, err = os.Lstat(current)
		}
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe symlink directory %s", current)
		}
		if !fi.IsDir() {
			return fmt.Errorf("path component %s is not a directory", current)
		}
	}
	return nil
}

func safeAbs(root, rel string) (string, string, string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", "", "", err
	}
	clean, err := cleanRelative(rel)
	if err != nil {
		return "", "", "", err
	}
	abs := filepath.Join(rootAbs, clean)
	relToRoot, err := filepath.Rel(rootAbs, abs)
	if err != nil {
		return "", "", "", err
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", "", "", fmt.Errorf("path %q escapes root", rel)
	}
	return rootAbs, abs, clean, nil
}

func cleanRelative(rel string) (string, error) {
	rel = filepath.FromSlash(rel)
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("absolute path is not allowed: %q", rel)
	}
	clean := filepath.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root: %q", rel)
	}
	return clean, nil
}
