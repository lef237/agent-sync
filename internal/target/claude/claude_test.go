package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lef237/agent-sync/internal/apply"
	"github.com/lef237/agent-sync/internal/discovery"
	"github.com/lef237/agent-sync/internal/planner"
)

func TestSyncE2E(t *testing.T) {
	root := t.TempDir()
	write(t, root, "AGENTS.md", "# Root instructions\n")
	write(t, root, "services/billing/AGENTS.md", "# Billing\n")
	write(t, root, ".agents/skills/review/SKILL.md", "---\nname: review\n---\n")
	write(t, root, ".agents/skills/release/SKILL.md", "---\nname: release\n---\n")

	src, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := New().Plan(root, src)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) == 0 {
		t.Fatal("expected plan actions")
	}

	st, err := apply.LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := apply.Apply(root, "claude", plan, st); err != nil {
		t.Fatal(err)
	}

	rootCLAUDE := read(t, root, "CLAUDE.md")
	if !strings.Contains(rootCLAUDE, "@AGENTS.md") {
		t.Fatalf("root CLAUDE.md missing import:\n%s", rootCLAUDE)
	}
	nestedCLAUDE := read(t, root, "services/billing/CLAUDE.md")
	if !strings.Contains(nestedCLAUDE, "@AGENTS.md") {
		t.Fatalf("nested CLAUDE.md missing import:\n%s", nestedCLAUDE)
	}
	for _, name := range []string{"review", "release"} {
		dst := filepath.Join(root, ".claude", "skills", name)
		fi, err := os.Lstat(dst)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s is not a symlink", dst)
		}
	}

	plan2, err := New().Plan(root, src)
	if err != nil {
		t.Fatal(err)
	}
	if !plan2.Empty() {
		t.Fatalf("expected idempotent plan, got %d actions", len(plan2.Actions))
	}
}

func TestSyncOverridePriority(t *testing.T) {
	root := t.TempDir()
	write(t, root, "AGENTS.md", "# Root\n")
	write(t, root, "AGENTS.override.md", "# Override\n")

	src, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := New().Plan(root, src)
	if err != nil {
		t.Fatal(err)
	}
	st, err := apply.LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := apply.Apply(root, "claude", plan, st); err != nil {
		t.Fatal(err)
	}
	content := read(t, root, "CLAUDE.md")
	if !strings.Contains(content, "@AGENTS.override.md") {
		t.Fatalf("expected override import:\n%s", content)
	}
	if strings.Contains(content, "@AGENTS.md") {
		t.Fatalf("unexpected base import:\n%s", content)
	}
}

func TestSyncStaleLinkRemoved(t *testing.T) {
	root := t.TempDir()
	write(t, root, "AGENTS.md", "# Root\n")
	write(t, root, ".agents/skills/review/SKILL.md", "---\nname: review\n---\n")

	src, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := New().Plan(root, src)
	if err != nil {
		t.Fatal(err)
	}
	st, err := apply.LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := apply.Apply(root, "claude", plan, st); err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(filepath.Join(root, ".agents", "skills", "review")); err != nil {
		t.Fatal(err)
	}
	src2, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	plan2, err := New().Plan(root, src2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".claude", "skills", "review")); err != nil {
		t.Fatal(err)
	}
	if err := apply.Apply(root, "claude", plan2, st); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".claude", "skills", "review")); !os.IsNotExist(err) {
		t.Fatalf("stale symlink not removed, err=%v", err)
	}
}

func TestSyncPreservesUserSkill(t *testing.T) {
	root := t.TempDir()
	write(t, root, "AGENTS.md", "# Root\n")
	write(t, root, ".agents/skills/review/SKILL.md", "---\nname: review\n---\n")
	write(t, root, ".claude/skills/claude-only/SKILL.md", "---\nname: claude-only\n---\n")

	src, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := New().Plan(root, src)
	if err != nil {
		t.Fatal(err)
	}
	st, err := apply.LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := apply.Apply(root, "claude", plan, st); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "claude-only", "SKILL.md")); err != nil {
		t.Fatalf("user skill lost: %v", err)
	}
}

func TestSyncPreservesUserSymlink(t *testing.T) {
	root := t.TempDir()
	write(t, root, "AGENTS.md", "# Root\n")
	write(t, root, ".agents/skills/review/SKILL.md", "---\nname: review\n---\n")
	userTarget := filepath.Join(root, "user-review")
	if err := os.MkdirAll(userTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(root, ".claude", "skills", "review")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(userTarget, dst); err != nil {
		t.Fatal(err)
	}

	src, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := New().Plan(root, src)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range plan.Actions {
		switch a := action.(type) {
		case planner.RemoveLink:
			if a.Path == ".claude/skills/review" {
				t.Fatalf("user symlink should not be removed: %v", action)
			}
		case planner.CreateLink:
			if a.Path == ".claude/skills/review" {
				t.Fatalf("user symlink should not be replaced: %v", action)
			}
		}
	}

	st, err := apply.LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := apply.Apply(root, "claude", plan, st); err != nil {
		t.Fatal(err)
	}
	cur, err := os.Readlink(dst)
	if err != nil {
		t.Fatal(err)
	}
	if cur != userTarget {
		t.Fatalf("user symlink target changed to %q", cur)
	}
}

func TestSyncBackfillsLegacyManagedSymlinkTarget(t *testing.T) {
	root := t.TempDir()
	write(t, root, "AGENTS.md", "# Root\n")
	write(t, root, ".agents/skills/review/SKILL.md", "---\nname: review\n---\n")
	dst := filepath.Join(root, ".claude", "skills", "review")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	srcDir := filepath.Join(root, ".agents", "skills", "review")
	target, err := filepath.Rel(filepath.Dir(dst), srcDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, dst); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(apply.StatePath(root), []byte(`{"version":1,"managedSymlinks":["review"]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	src, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := New().Plan(root, src)
	if err != nil {
		t.Fatal(err)
	}
	foundAdopt := false
	for _, action := range plan.Actions {
		switch a := action.(type) {
		case planner.AdoptLink:
			if a.Path == ".claude/skills/review" && a.Target == target {
				foundAdopt = true
			}
		case planner.CreateLink:
			if a.Path == ".claude/skills/review" {
				t.Fatalf("matching legacy link should be adopted, not recreated: %v", action)
			}
		}
	}
	if !foundAdopt {
		t.Fatalf("expected an adopt action, got %v", plan.Actions)
	}

	st, err := apply.LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := apply.Apply(root, "claude", plan, st); err != nil {
		t.Fatal(err)
	}
	cur, err := os.Readlink(dst)
	if err != nil {
		t.Fatal(err)
	}
	if cur != target {
		t.Fatalf("link target changed during adoption: %q", cur)
	}
	loaded, err := apply.LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Target("claude").ManagedSymlinks) != 1 || loaded.Target("claude").ManagedSymlinks[0].Name != "review" || loaded.Target("claude").ManagedSymlinks[0].Target != target {
		t.Fatalf("legacy target was not backfilled: %#v", loaded.Target("claude").ManagedSymlinks)
	}
}

func TestSyncDoesNotRemoveReplacedManagedSymlink(t *testing.T) {
	root := t.TempDir()
	write(t, root, "AGENTS.md", "# Root\n")
	write(t, root, ".agents/skills/review/SKILL.md", "---\nname: review\n---\n")

	src, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := New().Plan(root, src)
	if err != nil {
		t.Fatal(err)
	}
	st, err := apply.LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := apply.Apply(root, "claude", plan, st); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(root, ".claude", "skills", "review")
	userTarget := filepath.Join(root, "user-review")
	if err := os.MkdirAll(userTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dst); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(userTarget, dst); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, ".agents", "skills", "review")); err != nil {
		t.Fatal(err)
	}

	src2, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	plan2, err := New().Plan(root, src2)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range plan2.Actions {
		if a, ok := action.(planner.RemoveLink); ok && a.Path == ".claude/skills/review" {
			t.Fatalf("replaced user symlink should not be removed: %v", action)
		}
	}
	st2, err := apply.LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := apply.Apply(root, "claude", plan2, st2); err != nil {
		t.Fatal(err)
	}
	cur, err := os.Readlink(dst)
	if err != nil {
		t.Fatal(err)
	}
	if cur != userTarget {
		t.Fatalf("user symlink target changed to %q", cur)
	}
	st3, err := apply.LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(st3.Target("claude").ManagedSymlinks) != 0 {
		t.Fatalf("replaced link should be forgotten: %#v", st3.Target("claude").ManagedSymlinks)
	}
}

func TestSyncForgetsMissingManagedSymlink(t *testing.T) {
	root := t.TempDir()
	write(t, root, "AGENTS.md", "# Root\n")
	write(t, root, ".agents/skills/review/SKILL.md", "---\nname: review\n---\n")

	src, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := New().Plan(root, src)
	if err != nil {
		t.Fatal(err)
	}
	st, err := apply.LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := apply.Apply(root, "claude", plan, st); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, ".claude", "skills", "review")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, ".agents", "skills", "review")); err != nil {
		t.Fatal(err)
	}

	src2, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	plan2, err := New().Plan(root, src2)
	if err != nil {
		t.Fatal(err)
	}
	st2, err := apply.LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := apply.Apply(root, "claude", plan2, st2); err != nil {
		t.Fatal(err)
	}
	st3, err := apply.LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(st3.Target("claude").ManagedSymlinks) != 0 {
		t.Fatalf("missing link should be forgotten: %#v", st3.Target("claude").ManagedSymlinks)
	}
}

func TestSyncRejectsClaudeSymlink(t *testing.T) {
	root := t.TempDir()
	write(t, root, "AGENTS.md", "# Root\n")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}

	src, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New().Plan(root, src); err == nil {
		t.Fatal("Plan should reject a symlinked CLAUDE.md")
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("outside file was modified: %q", got)
	}
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A CLAUDE.md agent-sync generated used to keep importing an AGENTS.md that
// had been deleted, and --check still called the repository in sync.
func TestSyncRemovesGeneratedFileWhenAgentsFileDisappears(t *testing.T) {
	root := t.TempDir()
	write(t, root, "AGENTS.md", "# Root\n")
	write(t, root, "services/billing/AGENTS.md", "# Billing\n")
	syncOnce(t, root)

	if err := os.Remove(filepath.Join(root, "services", "billing", "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	syncOnce(t, root)

	if _, err := os.Stat(filepath.Join(root, "services", "billing", "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("generated CLAUDE.md should be withdrawn, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Fatalf("root CLAUDE.md should survive: %v", err)
	}

	st, err := apply.LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	files := st.Target("claude").ManagedFiles
	if len(files) != 1 || files[0] != "CLAUDE.md" {
		t.Fatalf("stale file still tracked: %v", files)
	}
	if plan := planOnce(t, root); !plan.Empty() {
		t.Fatalf("expected an empty plan after cleanup, got %v", plan.Actions)
	}
}

func TestSyncKeepsHandWrittenContentWhenAgentsFileDisappears(t *testing.T) {
	root := t.TempDir()
	write(t, root, "AGENTS.md", "# Root\n")
	write(t, root, "CLAUDE.md", "## Claude only\n\nkeep me\n")
	syncOnce(t, root)

	if err := os.Remove(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	syncOnce(t, root)

	content := read(t, root, "CLAUDE.md")
	if strings.Contains(content, planner.StartMarker) || strings.Contains(content, "@AGENTS.md") {
		t.Fatalf("managed block should be withdrawn:\n%s", content)
	}
	if !strings.Contains(content, "keep me") {
		t.Fatalf("hand-written content lost:\n%s", content)
	}
	if plan := planOnce(t, root); !plan.Empty() {
		t.Fatalf("expected an empty plan after cleanup, got %v", plan.Actions)
	}
}

// If the user removed the block themselves, agent-sync should let go of the
// file without rewriting it.
func TestSyncForgetsFileTheUserAlreadyCleaned(t *testing.T) {
	root := t.TempDir()
	write(t, root, "AGENTS.md", "# Root\n")
	syncOnce(t, root)

	if err := os.Remove(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	write(t, root, "CLAUDE.md", "hand written\n")
	syncOnce(t, root)

	if got := read(t, root, "CLAUDE.md"); got != "hand written\n" {
		t.Fatalf("file should be left alone, got %q", got)
	}
	st, err := apply.LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if files := st.Target("claude").ManagedFiles; len(files) != 0 {
		t.Fatalf("file should be forgotten: %v", files)
	}
}

func planOnce(t *testing.T, root string) *planner.Plan {
	t.Helper()
	src, err := discovery.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := New().Plan(root, src)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func syncOnce(t *testing.T, root string) {
	t.Helper()
	plan := planOnce(t, root)
	st, err := apply.LoadState(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := apply.Apply(root, "claude", plan, st); err != nil {
		t.Fatal(err)
	}
}
