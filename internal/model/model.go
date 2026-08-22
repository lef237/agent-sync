package model

type SourceState struct {
	Root         string
	Instructions []Instruction
	Skills       []Skill
}

type Instruction struct {
	Dir    string
	Source string
}

type Skill struct {
	Name string
	Src  string
	Dst  string
}

// StateVersion is the schema version this build writes.
//
// Version 1 kept a single flat managedSymlinks list, which only worked while
// claude was the one target: skill names are not unique across targets, so two
// targets syncing a skill of the same name would overwrite each other's
// ownership record and each target's cleanup pass would reach for the other's
// links.
const StateVersion = 2

// State records what agent-sync owns, keyed by target name.
type State struct {
	Version int                    `json:"version"`
	Targets map[string]TargetState `json:"targets"`
}

// TargetState is one target's ownership record.
type TargetState struct {
	// ManagedSymlinks are the links agent-sync created for this target.
	ManagedSymlinks []ManagedSymlink `json:"managedSymlinks,omitempty"`
	// ManagedFiles are the files whose agent-sync block this target
	// maintains, as repository-relative slash-separated paths. Tracking them
	// is what lets a block be withdrawn once its AGENTS.md is gone.
	ManagedFiles []string `json:"managedFiles,omitempty"`
}

// ManagedSymlink records the exact link target that agent-sync created.
// Target is empty only for legacy entries that have not been backfilled yet.
type ManagedSymlink struct {
	Name   string `json:"name"`
	Target string `json:"target"`
}

// Target returns the record for name, or a zero record when that target has
// never been synced.
func (s *State) Target(name string) TargetState {
	if s == nil {
		return TargetState{}
	}
	return s.Targets[name]
}

// SetTarget stores ts under name, dropping the entry entirely when the target
// owns nothing so an inactive target leaves no residue in the state file.
func (s *State) SetTarget(name string, ts TargetState) {
	if len(ts.ManagedSymlinks) == 0 && len(ts.ManagedFiles) == 0 {
		delete(s.Targets, name)
		return
	}
	if s.Targets == nil {
		s.Targets = make(map[string]TargetState, 1)
	}
	s.Targets[name] = ts
}

// Empty reports whether the state records no ownership at all.
func (s *State) Empty() bool { return s == nil || len(s.Targets) == 0 }
