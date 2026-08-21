package apply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lef237/agent-sync/internal/model"
	"github.com/lef237/agent-sync/internal/planner"
)

func TestLoadStateMigratesLegacySymlinkNames(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StatePath(root), []byte(`{"version":1,"managedSymlinks":["review"]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.ManagedSymlinks) != 1 || st.ManagedSymlinks[0].Name != "review" || st.ManagedSymlinks[0].Target != "" {
		t.Fatalf("unexpected migrated state: %#v", st.ManagedSymlinks)
	}
	if err := SaveState(root, st); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(StatePath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"name": "review"`) {
		t.Fatalf("state was not migrated: %s", b)
	}
}

func TestLoadStateRejectsMalformedState(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StatePath(root), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(root); err == nil {
		t.Fatal("LoadState should reject malformed JSON")
	}
}

func TestValidateOutputPathRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}

	if err := ValidateOutputPath(root, "CLAUDE.md"); err == nil {
		t.Fatal("ValidateOutputPath should reject a final symlink")
	}
	if got, err := os.ReadFile(outside); err != nil || string(got) != "original" {
		t.Fatalf("outside file changed: %q, %v", got, err)
	}
}

func TestApplyRecordsManagedLinkTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents", "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan := &planner.Plan{Actions: []planner.Action{
		planner.CreateLink{Path: ".claude/skills/review", Target: "../../.agents/skills/review"},
	}}
	st := &model.State{Version: 1}
	if err := Apply(root, plan, st); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.ManagedSymlinks) != 1 || loaded.ManagedSymlinks[0].Name != "review" || loaded.ManagedSymlinks[0].Target != "../../.agents/skills/review" {
		t.Fatalf("unexpected managed link state: %#v", loaded.ManagedSymlinks)
	}
}
