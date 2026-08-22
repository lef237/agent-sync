package planner

import (
	"fmt"
	"strings"
)

const (
	StartMarker = "<!-- agent-sync:start -->"
	EndMarker   = "<!-- agent-sync:end -->"
)

type Action interface {
	String() string
}

type Create struct {
	Path    string
	Content string
}

type Update struct {
	Path    string
	Content string
}

type CreateLink struct {
	Path   string
	Target string
}

// AdoptLink updates managed state for an existing link without changing the
// filesystem. It is used to migrate legacy state after verifying the target.
type AdoptLink struct {
	Path   string
	Target string
}

type RemoveLink struct {
	Path   string
	Target string
}

// ForgetLink removes a link from the managed state without touching the
// filesystem. It is used when a user has replaced or removed a previously
// managed link.
type ForgetLink struct {
	Path string
}

func (a Create) String() string     { return "create " + a.Path }
func (a Update) String() string     { return "update " + a.Path }
func (a CreateLink) String() string { return "link " + a.Path + " -> " + a.Target }
func (a AdoptLink) String() string  { return "adopt " + a.Path + " -> " + a.Target }
func (a RemoveLink) String() string { return "remove " + a.Path }
func (a ForgetLink) String() string { return "forget " + a.Path }

type Plan struct {
	Actions []Action
}

func (p *Plan) Add(a Action) { p.Actions = append(p.Actions, a) }

func (p *Plan) Empty() bool { return len(p.Actions) == 0 }

// blockRange is a half-open [start, end) span covering one managed block,
// including both markers.
type blockRange struct {
	start int
	end   int
}

// findManagedBlocks returns every managed block in s, in order. A stray end
// marker, or a start marker with no closing end marker, is reported as an
// error: guessing at a hand-broken file risks either swallowing the user's
// text or appending a fresh block on every run.
func findManagedBlocks(s string) ([]blockRange, error) {
	var blocks []blockRange
	for offset := 0; offset < len(s); {
		rest := s[offset:]
		startRel := strings.Index(rest, StartMarker)
		endRel := strings.Index(rest, EndMarker)
		switch {
		case startRel == -1 && endRel == -1:
			return blocks, nil
		case startRel == -1:
			return nil, fmt.Errorf("malformed managed block: %s appears without a preceding %s", EndMarker, StartMarker)
		case endRel == -1:
			return nil, fmt.Errorf("malformed managed block: %s is not closed by %s", StartMarker, EndMarker)
		case endRel < startRel:
			return nil, fmt.Errorf("malformed managed block: %s appears before %s", EndMarker, StartMarker)
		}
		blocks = append(blocks, blockRange{
			start: offset + startRel,
			end:   offset + endRel + len(EndMarker),
		})
		offset = blocks[len(blocks)-1].end
	}
	return blocks, nil
}

// ApplyManagedBlock returns existing with its managed block replaced by
// managed, and reports whether the result differs. Any extra managed blocks
// are dropped, so a file that somehow grew a second one converges back to
// exactly one.
func ApplyManagedBlock(existing, managed string) (string, bool, error) {
	blocks, err := findManagedBlocks(existing)
	if err != nil {
		return "", false, err
	}
	if len(blocks) == 0 {
		if strings.TrimSpace(existing) == "" {
			next := managed + "\n"
			return next, next != existing, nil
		}
		sep := "\n"
		if !strings.HasSuffix(existing, "\n") {
			sep = "\n\n"
		}
		next := existing + sep + managed + "\n"
		return next, next != existing, nil
	}

	before := existing[:blocks[0].start]
	after := cutBlocks(existing, blocks[0].end, blocks[1:])
	if before != "" && !strings.HasSuffix(before, "\n") {
		before += "\n"
	}
	if after != "" && !strings.HasPrefix(after, "\n") {
		after = "\n" + after
	}
	next := before + managed + after
	return next, next != existing, nil
}

// cutBlocks returns s from `from` to the end with every span in blocks
// removed, together with the newline that immediately follows each one so a
// removed block does not leave a stray blank line behind.
func cutBlocks(s string, from int, blocks []blockRange) string {
	var b strings.Builder
	prev := from
	for _, blk := range blocks {
		b.WriteString(s[prev:blk.start])
		prev = blk.end
		if prev < len(s) && s[prev] == '\n' {
			prev++
		}
	}
	b.WriteString(s[prev:])
	return b.String()
}
