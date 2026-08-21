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

type State struct {
	Version         int              `json:"version"`
	ManagedSymlinks []ManagedSymlink `json:"managedSymlinks"`
}

// ManagedSymlink records the exact link target that agent-sync created.
// Target is empty only for legacy entries that have not been backfilled yet.
type ManagedSymlink struct {
	Name   string `json:"name"`
	Target string `json:"target"`
}
