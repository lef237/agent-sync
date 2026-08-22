package planner

import (
	"strings"
	"testing"
)

const blockAGENTS = StartMarker + "\n@AGENTS.md\n" + EndMarker

func TestApplyManagedBlockInsert(t *testing.T) {
	existing := "## Claude specific\n\nsome notes\n"
	next, changed, err := ApplyManagedBlock(existing, blockAGENTS)
	if err != nil {
		t.Fatal(err)
	}
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
	next, changed, err := ApplyManagedBlock(existing, blockAGENTS)
	if err != nil {
		t.Fatal(err)
	}
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
	next, changed, err := ApplyManagedBlock(existing, blockAGENTS)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatalf("expected no change, got:\n%s", next)
	}
}

func TestApplyManagedBlockEmpty(t *testing.T) {
	next, changed, err := ApplyManagedBlock("", blockAGENTS)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || next != blockAGENTS+"\n" {
		t.Fatalf("expected block, got %q changed=%v", next, changed)
	}
}

// A reversed marker pair used to look like "no block at all", so every run
// appended another block and the file grew without ever converging.
func TestApplyManagedBlockRejectsReversedMarkers(t *testing.T) {
	existing := EndMarker + "\nmy notes\n" + StartMarker + "\n"
	if _, _, err := ApplyManagedBlock(existing, blockAGENTS); err == nil {
		t.Fatal("reversed markers must be reported, not appended to")
	}
}

func TestApplyManagedBlockRejectsStrayEndMarker(t *testing.T) {
	existing := "# Title\n\n" + EndMarker + "\n"
	if _, _, err := ApplyManagedBlock(existing, blockAGENTS); err == nil {
		t.Fatal("a stray end marker must be reported")
	}
}

func TestApplyManagedBlockRejectsUnterminatedBlock(t *testing.T) {
	existing := "# Title\n\n" + StartMarker + "\n@AGENTS.md\n"
	if _, _, err := ApplyManagedBlock(existing, blockAGENTS); err == nil {
		t.Fatal("an unterminated block must be reported")
	}
}

// Only the first block used to be rewritten, so a duplicate kept a stale
// import alive while the plan reported the file as up to date.
func TestApplyManagedBlockDropsDuplicateBlocks(t *testing.T) {
	override := StartMarker + "\n@AGENTS.override.md\n" + EndMarker
	existing := blockAGENTS + "\n\nnotes\n\n" + blockAGENTS + "\n"

	next, changed, err := ApplyManagedBlock(existing, override)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected change")
	}
	if strings.Count(next, StartMarker) != 1 {
		t.Fatalf("expected exactly one managed block:\n%s", next)
	}
	if strings.Contains(next, "@AGENTS.md") {
		t.Fatalf("stale import survived:\n%s", next)
	}
	if !strings.Contains(next, "notes") {
		t.Fatalf("user content lost:\n%s", next)
	}

	again, changed, err := ApplyManagedBlock(next, override)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatalf("expected idempotence, got:\n%s", again)
	}
}
