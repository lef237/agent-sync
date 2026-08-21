package target

import (
	"agent-sync/internal/model"
	"agent-sync/internal/planner"
)

type Target interface {
	Name() string
	Plan(root string, src *model.SourceState) (*planner.Plan, error)
}