package target

import (
	"github.com/lef237/agent-sync/internal/model"
	"github.com/lef237/agent-sync/internal/planner"
)

type Target interface {
	Name() string
	Plan(root string, src *model.SourceState) (*planner.Plan, error)
}