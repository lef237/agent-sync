package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-sync/internal/apply"
	"agent-sync/internal/discovery"
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

	st := apply.LoadState(root)
	if err := apply.Apply(root, plan, st); err != nil {
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
	st := apply.LoadState(root)
	if err := apply.Apply(root, plan, st); err != nil {
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
	st := apply.LoadState(root)
	if err := apply.Apply(root, plan, st); err != nil {
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
	if err := apply.Apply(root, plan2, st); err != nil {
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
	st := apply.LoadState(root)
	if err := apply.Apply(root, plan, st); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "claude-only", "SKILL.md")); err != nil {
		t.Fatalf("user skill lost: %v", err)
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