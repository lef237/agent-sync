package model

type SourceState struct {
	Root          string
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
	Version         int      `json:"version"`
	ManagedSymlinks []string `json:"managedSymlinks"`
}