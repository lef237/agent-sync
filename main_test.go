package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func TestVerboseReportsAnUpToDateRepository(t *testing.T) {
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
	quiet := captureStdout(t, func() {
		if code := run([]string{"claude"}); code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
	})
	if quiet != "" {
		t.Fatalf("an up-to-date sync should be quiet, got %q", quiet)
	}
	loud := captureStdout(t, func() {
		if code := run([]string{"claude", "--verbose"}); code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
	})
	if !strings.Contains(loud, "already in sync") || !strings.Contains(loud, "root:") {
		t.Fatalf("--verbose produced no extra output: %q", loud)
	}
}

func TestHelpGoesToStdoutAndErrorsDoNot(t *testing.T) {
	help := captureStdout(t, func() {
		if code := run([]string{"--help"}); code != 0 {
			t.Fatalf("expected --help to return 0, got %d", code)
		}
	})
	if !strings.Contains(help, "usage: agent-sync") {
		t.Fatalf("--help should print usage on stdout, got %q", help)
	}

	bad := captureStdout(t, func() {
		if code := run([]string{"--nope"}); code != 2 {
			t.Fatalf("expected exit 2 for an unknown flag, got %d", code)
		}
	})
	if bad != "" {
		t.Fatalf("a parse error must not write to stdout, got %q", bad)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	os.Stdout = orig
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out := <-done
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return out
}
