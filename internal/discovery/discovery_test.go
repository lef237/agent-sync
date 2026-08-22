package discovery

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lef237/agent-sync/internal/model"
)

func TestRepoRootAcceptsGitFile(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "services", "billing")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /tmp/worktree\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := RepoRoot(child)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("RepoRoot() = %q, want %q", got, root)
	}
}

// Build output names used to be skipped at every depth, which silently
// dropped instructions from ordinary source directories -- internal/target in
// this very repository, or packages/build in a monorepo.
func TestDiscoverSkipsBuildNamesOnlyAtTheRoot(t *testing.T) {
	root := t.TempDir()
	nested := []string{
		"AGENTS.md",
		"internal/target/AGENTS.md",
		"packages/build/AGENTS.md",
		"packages/dist/AGENTS.md",
		"packages/out/AGENTS.md",
		"packages/vendor/AGENTS.md",
	}
	skippedAtRootPaths := []string{
		"build/AGENTS.md",
		"dist/AGENTS.md",
		"node_modules/pkg/AGENTS.md",
		".venv/lib/AGENTS.md",
	}
	for _, rel := range append(append([]string{}, nested...), skippedAtRootPaths...) {
		writeFile(t, root, rel)
	}

	src, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, in := range src.Instructions {
		got[in.Source] = true
	}
	for _, rel := range nested {
		if !got[rel] {
			t.Errorf("%s should have been discovered, got %v", rel, sources(src))
		}
	}
	for _, rel := range skippedAtRootPaths {
		if got[rel] {
			t.Errorf("%s should have been skipped, got %v", rel, sources(src))
		}
	}
}

// A single unreadable directory used to abort the whole run.
func TestDiscoverSkipsUnreadableDirectories(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read anything")
	}
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md")
	secret := filepath.Join(root, "secret")
	if err := os.MkdirAll(secret, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(secret, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(secret, 0o755) })

	src, err := Discover(root)
	if err != nil {
		t.Fatalf("one unreadable directory must not fail discovery: %v", err)
	}
	if len(src.Instructions) != 1 {
		t.Fatalf("expected the readable AGENTS.md, got %v", sources(src))
	}
	if len(src.Warnings) != 1 || !strings.Contains(src.Warnings[0], "secret") {
		t.Fatalf("expected a warning about the unreadable directory, got %v", src.Warnings)
	}
	if !src.DiscoveryIncomplete {
		t.Fatal("unreadable directories must mark discovery incomplete")
	}
}

func TestDiscoverWarnsAboutNestedSkills(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md")
	writeFile(t, root, ".agents/skills/review/SKILL.md")
	writeFile(t, root, "sub/.agents/skills/nested/SKILL.md")

	src, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(src.Skills) != 1 || src.Skills[0].Name != "review" {
		t.Fatalf("only root skills are synced, got %v", src.Skills)
	}
	if len(src.Warnings) != 1 || !strings.Contains(src.Warnings[0], "sub/.agents/skills") {
		t.Fatalf("expected a warning about the nested skills, got %v", src.Warnings)
	}
	if src.DiscoveryIncomplete {
		t.Fatal("intentional exclusions must not mark discovery incomplete")
	}
}

func TestDiscoverSkipsGitignoredFiles(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git unavailable: %v: %s", err, out)
	}
	writeFile(t, root, "AGENTS.md")
	writeFile(t, root, "tmpwork/pkg/AGENTS.md")
	writeFile(t, root, "forced/AGENTS.md")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("tmpwork/\nforced/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A file that is ignored by a rule but tracked anyway must still count.
	if out, err := exec.Command("git", "-C", root, "add", "-f", "forced/AGENTS.md").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}

	src, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	got := sources(src)
	want := []string{"AGENTS.md", "forced/AGENTS.md"}
	if len(got) != len(want) {
		t.Fatalf("discovered %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("discovered %v, want %v", got, want)
		}
	}
}

func sources(src *model.SourceState) []string {
	out := make([]string, 0, len(src.Instructions))
	for _, in := range src.Instructions {
		out = append(out, in.Source)
	}
	return out
}

func writeFile(t *testing.T, root, rel string) {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
