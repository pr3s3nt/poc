package tfexec

import (
	"context"
	"encoding/json"
	"sync"

	"orchestrator/internal/application/envops"
	"orchestrator/internal/ports/terraformexec"
)

// ReadinessDetailPrefix keys the safe readiness observation of one resource in
// the owning Environment operation detail (UC-08 BR-11, issue #68).
const ReadinessDetailPrefix = "readiness:"

// ReadinessDetail is the persisted, value-free wait state of one resource.
type ReadinessDetail struct {
	DeploymentID string                         `json:"deploymentId,omitempty"`
	Waiting      []terraformexec.PodWait        `json:"waiting"`
	Replaced     []terraformexec.PodReplacement `json:"replaced,omitempty"`
}

// readinessRecorder persists the latest Pod wait reasons and every stuck Pod
// replacement on the operation that owns ctx. It writes only on change, and a
// failed write never changes the readiness verdict.
func readinessRecorder(ctx context.Context, deploymentID, descriptor string) func(terraformexec.ReadinessObservation) {
	var mu sync.Mutex
	detail := ReadinessDetail{DeploymentID: deploymentID, Waiting: []terraformexec.PodWait{}}
	last := ""
	return func(o terraformexec.ReadinessObservation) {
		mu.Lock()
		defer mu.Unlock()
		if o.Waiting != nil {
			detail.Waiting = o.Waiting
		}
		if o.Replaced != nil {
			detail.Replaced = append(detail.Replaced, *o.Replaced)
		}
		// Elapsed seconds alone are not a change worth a write.
		key := detail
		key.Waiting = make([]terraformexec.PodWait, len(detail.Waiting))
		for i, w := range detail.Waiting {
			w.StuckSeconds = 0
			key.Waiting[i] = w
		}
		raw, _ := json.Marshal(key)
		if string(raw) == last {
			return
		}
		last = string(raw)
		_ = envops.Annotate(ctx, ReadinessDetailPrefix+descriptor, detail)
	}
}
