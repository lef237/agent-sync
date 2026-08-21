package planner

import (
	"strings"
	"testing"
)

const blockAGENTS = StartMarker + "\n@AGENTS.md\n" + EndMarker

func TestApplyManagedBlockInsert(t *testing.T) {
	existing := "## Claude specific\n\nsome notes\n"
	next, changed := ApplyManagedBlock(existing, blockAGENTS)
	if !changed {
		t.Fatal("expected change")
	}
	if !strings.HasPrefix(next, "## Claude specific") {
		t.Fatalf("user content lost:\n%s", next)
	}
	if !strings.Contains(next, "@AGENTS.md") {
		t.Fatalf("import missing:\n%s", next)
	}
}

func TestApplyManagedBlockReplace(t *testing.T) {
	existing := "# Title\n\n" + StartMarker + "\n@AGENTS.override.md\n" + EndMarker + "\n\ncustom tail\n"
	next, changed := ApplyManagedBlock(existing, blockAGENTS)
	if !changed {
		t.Fatal("expected change")
	}
	if strings.Contains(next, "AGENTS.override.md") {
		t.Fatalf("old import not replaced:\n%s", next)
	}
	if !strings.Contains(next, "custom tail") {
		t.Fatalf("user content lost:\n%s", next)
	}
	if !strings.HasPrefix(next, "# Title") {
		t.Fatalf("header lost:\n%s", next)
	}
}

func TestApplyManagedBlockNoop(t *testing.T) {
	existing := "# Title\n\n" + blockAGENTS + "\ncustom tail\n"
	next, changed := ApplyManagedBlock(existing, blockAGENTS)
	if changed {
		t.Fatalf("expected no change, got:\n%s", next)
	}
}

func TestApplyManagedBlockEmpty(t *testing.T) {
	next, changed := ApplyManagedBlock("", blockAGENTS)
	if !changed || next != blockAGENTS+"\n" {
		t.Fatalf("expected block, got %q changed=%v", next, changed)
	}
}