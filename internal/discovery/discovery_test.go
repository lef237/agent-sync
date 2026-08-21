package discovery

import (
	"os"
	"path/filepath"
	"testing"
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
