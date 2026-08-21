package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckFlagAfterSubcommand(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	code := run([]string{"claude", "--check"})
	if code != 1 {
		t.Fatalf("expected exit 1 (change detected), got %d", code)
	}
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("--check must not apply changes; CLAUDE.md exists, err=%v", err)
	}

	code = run([]string{"--check", "claude"})
	if code != 1 {
		t.Fatalf("expected exit 1 for flags-first order, got %d", code)
	}
}

func TestApplyCreatesFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	if code := run([]string{"claude"}); code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Fatalf("CLAUDE.md should exist: %v", err)
	}
}

func TestHelpReturnsSuccess(t *testing.T) {
	if code := run([]string{"--help"}); code != 0 {
		t.Fatalf("expected --help to return 0, got %d", code)
	}
}

func TestRejectsExtraPositionalArguments(t *testing.T) {
	if code := run([]string{"claude", "unexpected"}); code != 2 {
		t.Fatalf("expected extra positional argument to return 2, got %d", code)
	}
}
