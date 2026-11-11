package envops

import (
	"context"
	"encoding/json"
	"sync"

	"orchestrator/internal/domain/workloadwait"
)

// WaitRecorder persists the latest Pod wait reasons and every stuck Pod
// replacement of one resource or workload on the operation that owns ctx
// (UC-08 BR-11, UC-06 CD-11). It writes only on change; a failed write never
// changes the outcome of the wait.
func WaitRecorder(ctx context.Context, key, deploymentID string) func(waiting []workloadwait.PodWait, replaced *workloadwait.PodReplacement) {
	var mu sync.Mutex
	detail := workloadwait.Detail{DeploymentID: deploymentID, Waiting: []workloadwait.PodWait{}}
	last := ""
	return func(waiting []workloadwait.PodWait, replaced *workloadwait.PodReplacement) {
		mu.Lock()
		defer mu.Unlock()
		if waiting != nil {
			detail.Waiting = waiting
		}
		if replaced != nil {
			detail.Replaced = append(detail.Replaced, *replaced)
		}
		// Elapsed seconds alone are not a change worth a write.
		probe := detail
		probe.Waiting = make([]workloadwait.PodWait, len(detail.Waiting))
		for i, w := range detail.Waiting {
			w.StuckSeconds = 0
			probe.Waiting[i] = w
		}
		raw, _ := json.Marshal(probe)
		if string(raw) == last {
			return
		}
		last = string(raw)
		_ = Annotate(ctx, key, detail)
	}
}
