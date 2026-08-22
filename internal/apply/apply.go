package apply

import (
	"bytes"
	"encoding/json"
	"errors"
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
		return &model.State{Version: model.StateVersion}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", StatePath(root), err)
	}

	st, err := decodeState(b)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", StatePath(root), err)
	}
	if err := validateState(st); err != nil {
		return nil, fmt.Errorf("invalid state %s: %w", StatePath(root), err)
	}
	return st, nil
}

// legacyTarget owns everything recorded by version 1, which predates targets
// and could only ever have described the claude adapter.
const legacyTarget = "claude"

// decodeState reads every state layout agent-sync has written: version 2 with
// per-target records, version 1 with a flat managedSymlinks list, and the
// original version 1 form where that list held bare skill names.
func decodeState(b []byte) (*model.State, error) {
	var raw struct {
		Version         int                          `json:"version"`
		Targets         map[string]model.TargetState `json:"targets"`
		ManagedSymlinks json.RawMessage              `json:"managedSymlinks"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	if raw.Version == 0 {
		raw.Version = 1
	}
	if raw.Version > model.StateVersion {
		return nil, fmt.Errorf("unsupported state version %d (this build writes %d)", raw.Version, model.StateVersion)
	}

	st := &model.State{Version: model.StateVersion, Targets: raw.Targets}
	if len(raw.Targets) == 0 {
		links, err := decodeManagedSymlinks(raw.ManagedSymlinks)
		if err != nil {
			return nil, err
		}
		st.SetTarget(legacyTarget, model.TargetState{ManagedSymlinks: links})
	}
	return st, nil
}

func decodeManagedSymlinks(raw json.RawMessage) ([]model.ManagedSymlink, error) {
	data := bytes.TrimSpace(raw)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil, nil
	}

	var links []model.ManagedSymlink
	if err := json.Unmarshal(data, &links); err == nil {
		return links, nil
	}

	var legacy []string
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("managedSymlinks must be an array of managed links: %w", err)
	}
	links = nil
	for _, name := range legacy {
		links = append(links, model.ManagedSymlink{Name: name})
	}
	return links, nil
}

func validateState(st *model.State) error {
	for name, ts := range st.Targets {
		if name == "" {
			return fmt.Errorf("empty target name")
		}
		seen := make(map[string]bool, len(ts.ManagedSymlinks))
		for _, link := range ts.ManagedSymlinks {
			if !validSkillName(link.Name) {
				return fmt.Errorf("target %q: invalid managed skill name %q", name, link.Name)
			}
			if seen[link.Name] {
				return fmt.Errorf("target %q: duplicate managed skill name %q", name, link.Name)
			}
			seen[link.Name] = true
		}

		seenFiles := make(map[string]bool, len(ts.ManagedFiles))
		for _, path := range ts.ManagedFiles {
			if err := validManagedFile(path); err != nil {
				return fmt.Errorf("target %q: %w", name, err)
			}
			if seenFiles[path] {
				return fmt.Errorf("target %q: duplicate managed file %q", name, path)
			}
			seenFiles[path] = true
		}
	}
	return nil
}

// validManagedFile requires a normalized, slash-separated path inside the
// repository, so a hand-edited state file cannot point the cleanup pass at
// something outside it.
func validManagedFile(path string) error {
	clean, err := cleanRelative(path)
	if err != nil {
		return fmt.Errorf("invalid managed file %q: %w", path, err)
	}
	if filepath.ToSlash(clean) != path {
		return fmt.Errorf("managed file %q is not a normalized relative path", path)
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
	b, err := encodeState(st)
	if err != nil {
		return err
	}
	return writeState(root, b)
}

// saveStateIfChanged writes the state file only when its contents would
// actually differ. A repository with nothing to track should not grow a
// .claude directory just because agent-sync ran.
func saveStateIfChanged(root string, st *model.State) error {
	next, err := encodeState(st)
	if err != nil {
		return err
	}
	relPath := filepath.ToSlash(filepath.Join(".claude", stateFileName))
	if err := ValidateOutputPath(root, relPath); err != nil {
		return err
	}

	current, err := os.ReadFile(StatePath(root))
	switch {
	case os.IsNotExist(err):
		if st.Empty() {
			return nil
		}
	case err != nil:
		return fmt.Errorf("read %s: %w", StatePath(root), err)
	case bytes.Equal(current, next):
		return nil
	}
	return writeState(root, next)
}

func encodeState(st *model.State) ([]byte, error) {
	if st == nil {
		return nil, fmt.Errorf("cannot save a nil state")
	}
	if st.Version == 0 {
		st.Version = model.StateVersion
	}
	if st.Version != model.StateVersion {
		return nil, fmt.Errorf("cannot write state version %d (this build writes %d)", st.Version, model.StateVersion)
	}
	if err := validateState(st); err != nil {
		return nil, err
	}

	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func writeState(root string, b []byte) error {
	relPath := filepath.ToSlash(filepath.Join(".claude", stateFileName))
	return writeAtomic(root, relPath, b, 0o644, false)
}

// Apply carries out plan on behalf of the named target and records what the
// target now owns. Ownership is tracked per target because skill names are
// only unique within one.
func Apply(root, targetName string, plan *planner.Plan, st *model.State) error {
	if st == nil {
		return fmt.Errorf("cannot apply a plan with nil state")
	}
	if targetName == "" {
		return fmt.Errorf("cannot apply a plan without a target name")
	}

	ts := st.Target(targetName)
	managed := make(map[string]string, len(ts.ManagedSymlinks))
	for _, link := range ts.ManagedSymlinks {
		managed[link.Name] = link.Target
	}
	files := make(map[string]bool, len(ts.ManagedFiles)+len(plan.KeptFiles))
	for _, path := range ts.ManagedFiles {
		files[path] = true
	}
	// Files the plan verified as already correct are ours regardless of how
	// the rest of the plan goes.
	for _, path := range plan.KeptFiles {
		files[path] = true
	}

	var applyErr error
	for _, a := range plan.Actions {
		if err := applyAction(root, a, managed, files); err != nil {
			applyErr = err
			break
		}
	}

	ts.ManagedFiles = make([]string, 0, len(files))
	for path := range files {
		ts.ManagedFiles = append(ts.ManagedFiles, path)
	}
	sort.Strings(ts.ManagedFiles)

	ts.ManagedSymlinks = make([]model.ManagedSymlink, 0, len(managed))
	for name, target := range managed {
		ts.ManagedSymlinks = append(ts.ManagedSymlinks, model.ManagedSymlink{
			Name:   name,
			Target: target,
		})
	}
	sort.Slice(ts.ManagedSymlinks, func(i, j int) bool {
		return ts.ManagedSymlinks[i].Name < ts.ManagedSymlinks[j].Name
	})
	st.SetTarget(targetName, ts)

	// Persist what actually happened even when an action failed. Links created
	// before the failure would otherwise be left on disk with no recorded
	// owner, and every later run would mistake them for user-created links.
	if err := saveStateIfChanged(root, st); err != nil {
		if applyErr != nil {
			return errors.Join(applyErr, err)
		}
		return err
	}
	return applyErr
}

func applyAction(root string, a planner.Action, managed map[string]string, files map[string]bool) error {
	switch act := a.(type) {
	case planner.Create:
		if err := writeFile(root, act.Path, act.Content, true); err != nil {
			return err
		}
		files[act.Path] = true
	case planner.Update:
		if err := writeFile(root, act.Path, act.Content, false); err != nil {
			return err
		}
		files[act.Path] = true
	case planner.StripFile:
		if err := writeFile(root, act.Path, act.Content, false); err != nil {
			return err
		}
		delete(files, act.Path)
	case planner.RemoveFile:
		if err := removeManagedFile(root, act.Path); err != nil {
			return err
		}
		delete(files, act.Path)
	case planner.ForgetFile:
		delete(files, act.Path)
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
	return nil
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

// removeManagedFile deletes a file agent-sync fully owned. It re-checks that
// nothing but the managed block is in there, so content written between
// planning and applying is never thrown away.
func removeManagedFile(root, path string) error {
	if err := ValidateOutputPath(root, path); err != nil {
		return err
	}
	_, abs, _, err := safeAbs(root, path)
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
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink; remove it manually", abs)
	}
	if fi.IsDir() {
		return fmt.Errorf("%s is a directory", abs)
	}

	b, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	rest, _, err := planner.StripManagedBlock(string(b))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if strings.TrimSpace(rest) != "" {
		return fmt.Errorf("%s gained content since planning; leave it untouched", abs)
	}
	return os.Remove(abs)
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
