package planner

import "strings"

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

type RemoveLink struct {
	Path string
}

func (a Create) String() string     { return "create " + a.Path }
func (a Update) String() string     { return "update " + a.Path }
func (a CreateLink) String() string { return "link " + a.Path + " -> " + a.Target }
func (a RemoveLink) String() string { return "remove " + a.Path }

type Plan struct {
	Actions []Action
}

func (p *Plan) Add(a Action) { p.Actions = append(p.Actions, a) }

func (p *Plan) Empty() bool { return len(p.Actions) == 0 }

func ApplyManagedBlock(existing, managed string) (string, bool) {
	start := strings.Index(existing, StartMarker)
	end := strings.Index(existing, EndMarker)
	if start == -1 || end == -1 || end < start {
		if strings.TrimSpace(existing) == "" {
			next := managed + "\n"
			return next, next != existing
		}
		sep := "\n"
		if !strings.HasSuffix(existing, "\n") {
			sep = "\n\n"
		}
		next := existing + sep + managed + "\n"
		return next, next != existing
	}
	before := existing[:start]
	after := existing[end+len(EndMarker):]
	if before != "" && !strings.HasSuffix(before, "\n") {
		before += "\n"
	}
	if after != "" && !strings.HasPrefix(after, "\n") {
		after = "\n" + after
	}
	next := before + managed + after
	return next, next != existing
}