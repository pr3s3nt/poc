// Package catalogsource defines discovery-only Git reads; execution never uses it.
package catalogsource

import (
	"context"
	"orchestrator/internal/domain/catalogrelease"
	"time"
)

type Tag struct {
	Name     string
	Commit   string
	InBranch bool
}
type Observation struct {
	BranchTip  string
	Tags       []Tag
	ObservedAt time.Time
}
type Discovery interface {
	Discover(context.Context, catalogrelease.Source) (Observation, error)
}

// Inspector must verify the complete manifest and all components at the supplied
// exact commit. Cache preparation alone never publishes a release.
type Inspector interface {
	InspectRelease(context.Context, catalogrelease.Source, Tag, Observation) (catalogrelease.Release, error)
}
