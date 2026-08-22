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
	if len(st.Target("claude").ManagedSymlinks) != 1 || st.Target("claude").ManagedSymlinks[0].Name != "review" || st.Target("claude").ManagedSymlinks[0].Target != "" {
		t.Fatalf("unexpected migrated state: %#v", st.Target("claude").ManagedSymlinks)
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
	st := &model.State{}
	if err := Apply(root, "claude", plan, st); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Target("claude").ManagedSymlinks) != 1 || loaded.Target("claude").ManagedSymlinks[0].Name != "review" || loaded.Target("claude").ManagedSymlinks[0].Target != "../../.agents/skills/review" {
		t.Fatalf("unexpected managed link state: %#v", loaded.Target("claude").ManagedSymlinks)
	}
}

// A mid-plan failure used to skip SaveState entirely, so links created before
// the failure stayed on disk with no recorded owner and every later run
// treated them as user-created.
func TestApplyPersistsStateWhenAnActionFails(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"aaa", "zzz"} {
		if err := os.MkdirAll(filepath.Join(root, ".agents", "skills", name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Occupy the second destination so its CreateLink fails.
	if err := os.MkdirAll(filepath.Join(root, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".claude", "skills", "zzz"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan := &planner.Plan{Actions: []planner.Action{
		planner.CreateLink{Path: ".claude/skills/aaa", Target: "../../.agents/skills/aaa"},
		planner.CreateLink{Path: ".claude/skills/zzz", Target: "../../.agents/skills/zzz"},
	}}
	if err := Apply(root, "claude", plan, &model.State{}); err == nil {
		t.Fatal("expected the second CreateLink to fail")
	}

	if _, err := os.Lstat(filepath.Join(root, ".claude", "skills", "aaa")); err != nil {
		t.Fatalf("first link should have been created: %v", err)
	}
	loaded, err := LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Target("claude").ManagedSymlinks) != 1 || loaded.Target("claude").ManagedSymlinks[0].Name != "aaa" {
		t.Fatalf("the created link must stay owned by agent-sync: %#v", loaded.Target("claude").ManagedSymlinks)
	}
}

func TestApplyLeavesStateFileAloneWhenThereIsNothingToOwn(t *testing.T) {
	root := t.TempDir()
	if err := Apply(root, "claude", &planner.Plan{}, &model.State{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("a repository with nothing to own should not gain a .claude directory, err=%v", err)
	}
}

func TestApplyRecordsGeneratedFiles(t *testing.T) {
	root := t.TempDir()
	plan := &planner.Plan{
		Actions:   []planner.Action{planner.Create{Path: "CLAUDE.md", Content: "hi\n"}},
		KeptFiles: []string{"services/billing/CLAUDE.md"},
	}
	if err := Apply(root, "claude", plan, &model.State{}); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.Target("claude").ManagedFiles
	want := []string{"CLAUDE.md", "services/billing/CLAUDE.md"}
	if len(got) != len(want) {
		t.Fatalf("managed files = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("managed files = %v, want %v", got, want)
		}
	}
}

func TestApplyDoesNotRewriteAnUnchangedStateFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents", "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan := &planner.Plan{Actions: []planner.Action{
		planner.CreateLink{Path: ".claude/skills/review", Target: "../../.agents/skills/review"},
	}}
	st := &model.State{}
	if err := Apply(root, "claude", plan, st); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(StatePath(root))
	if err != nil {
		t.Fatal(err)
	}

	st2, err := LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(root, "claude", &planner.Plan{}, st2); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(StatePath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("an unchanged state file should not be rewritten")
	}
}

func TestLoadStateMigratesVersion1IntoTheClaudeTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	v1 := `{"version":1,"managedSymlinks":[{"name":"review","target":"../../.agents/skills/review"}]}`
	if err := os.WriteFile(StatePath(root), []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	links := st.Target("claude").ManagedSymlinks
	if len(links) != 1 || links[0].Name != "review" || links[0].Target != "../../.agents/skills/review" {
		t.Fatalf("version 1 state was not migrated: %#v", st)
	}
	if st.Version != model.StateVersion {
		t.Fatalf("expected version %d, got %d", model.StateVersion, st.Version)
	}
}

func TestLoadStateRejectsAFutureVersion(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	future := `{"version":99,"targets":{}}`
	if err := os.WriteFile(StatePath(root), []byte(future), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(root); err == nil {
		t.Fatal("a state written by a newer build must not be silently downgraded")
	}
}

// Skill names are only unique within a target, so two targets syncing a skill
// of the same name must not overwrite each other's ownership record.
func TestApplyKeepsTargetsIndependent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents", "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	st := &model.State{}
	if err := Apply(root, "claude", &planner.Plan{Actions: []planner.Action{
		planner.CreateLink{Path: ".claude/skills/review", Target: "../../.agents/skills/review"},
	}}, st); err != nil {
		t.Fatal(err)
	}
	if err := Apply(root, "cursor", &planner.Plan{Actions: []planner.Action{
		planner.CreateLink{Path: ".cursor/skills/review", Target: "../../.agents/skills/review"},
	}}, st); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"claude", "cursor"} {
		links := loaded.Target(name).ManagedSymlinks
		if len(links) != 1 || links[0].Name != "review" {
			t.Fatalf("target %q lost its record: %#v", name, loaded.Targets)
		}
	}
}

// A link recorded by an older build on Windows holds backslashes. Comparing
// it byte for byte against the slash-separated target agent-sync now plans
// would read as "changed outside agent-sync".
func TestSameLinkTargetIgnoresSeparatorStyle(t *testing.T) {
	if !SameLinkTarget(`..\..\.agents\skills\review`, "../../.agents/skills/review") {
		t.Fatal("separator style should not make two targets differ")
	}
	if SameLinkTarget("../../.agents/skills/review", "../../.agents/skills/release") {
		t.Fatal("different targets must still differ")
	}
}

func TestCreateReportsAPathThatAppearedDuringApply(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("appeared\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Apply(root, "claude", &planner.Plan{Actions: []planner.Action{
		planner.Create{Path: "CLAUDE.md", Content: "new\n"},
	}}, &model.State{})
	if err == nil {
		t.Fatal("expected Create to refuse an occupied path")
	}
	if !strings.Contains(err.Error(), "appeared while applying") {
		t.Fatalf("error should explain what happened, got %v", err)
	}
	if strings.Contains(err.Error(), ".agent-sync-") {
		t.Fatalf("error should not leak the temporary file name, got %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "CLAUDE.md")); err != nil || string(got) != "appeared\n" {
		t.Fatalf("existing file must be left alone: %q, %v", got, err)
	}
}
