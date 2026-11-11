package fleet

import (
	"context"
	"encoding/json"
	k8s "orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/domain/progress"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
	"sort"
	"time"
)

// collectProgress runs inside the existing readiness observation. It is read-only
// and never influences health/ownership decisions. Only exact-revision owned
// controllers and their current-revision Pods contribute diagnostic facts.
func (p *Provider) collectProgress(ctx context.Context, i port.Intent, r port.Receipt) (pods []progress.Pod, events []progress.Event) {
	if p.Manager == nil {
		return
	}
	cli, close, err := k8s.OpenCLI(ctx, p.Manager.Verifier.KubectlPath, p.Manager.Verifier.Credentials, i.Target)
	if err != nil {
		return
	}
	defer close()
	return collectReadinessProgress(ctx, cli, i, r)
}

func collectReadinessProgress(ctx context.Context, cli k8s.CLI, i port.Intent, r port.Receipt) (pods []progress.Pod, events []progress.Event) {
	var refs []execution.ObjectRef
	if json.Unmarshal([]byte(r.Handle["inventory"]), &refs) != nil {
		return
	}
collect:
	for _, ref := range refs {
		if ref.Kind != "Deployment" && ref.Kind != "StatefulSet" {
			continue
		}
		object, found, err := cli.Get(ctx, i.Target.Namespace, ref.Kind, ref.Name)
		if err != nil || !found {
			continue
		}
		md, _ := object["metadata"].(map[string]any)
		a, _ := md["annotations"].(map[string]any)
		if a["orchestrator.io/delivery-revision"] != i.Revision.ID || a["meta.helm.sh/release-name"] != r.Handle["release"] {
			continue
		}
		labels, _ := md["labels"].(map[string]any)
		workload, _ := labels["orchestrator.io/workload"].(string)
		selector, ok := matchLabels(object)
		if !ok {
			continue
		}
		revision, ok, err := currentPodRevision(ctx, cli, i.Target.Namespace, ref.Kind, object, selector)
		if err != nil || !ok {
			continue
		}
		items, err := listItems(ctx, cli, i.Target.Namespace, "pods", selector+","+revision)
		if err != nil {
			continue
		}
		for _, pod := range items {
			if len(pods) >= 100 {
				break collect
			}
			pm, _ := pod["metadata"].(map[string]any)
			name, _ := pm["name"].(string)
			uid, _ := pm["uid"].(string)
			status, _ := pod["status"].(map[string]any)
			phase, _ := status["phase"].(string)
			seen := false
			for _, key := range []string{"initContainerStatuses", "containerStatuses"} {
				cs, _ := status[key].([]any)
				for _, raw := range cs {
					c, _ := raw.(map[string]any)
					im, _ := c["image"].(string)
					n, _ := c["restartCount"].(float64)
					reason := phase
					for _, stateKey := range []string{"lastState", "state"} {
						state, _ := c[stateKey].(map[string]any)
						for _, kind := range []string{"terminated", "waiting"} {
							v, _ := state[kind].(map[string]any)
							if code, ok := v["reason"].(string); ok && code != "" {
								reason = code
							}
						}
					}
					pods = append(pods, progress.Pod{Phase: phase, Workload: workload, Name: name, Image: im, Reason: progress.Reason(reason), Restarts: int(n)})
					seen = true
				}
			}
			if !seen {
				pods = append(pods, progress.Pod{Phase: phase, Workload: workload, Name: name, Reason: progress.Reason(phase)})
			}
			if uid == "" {
				continue
			}
			raw, err := cli.Run(ctx, nil, "get", "events", "-n", i.Target.Namespace, "--field-selector", "involvedObject.uid="+uid, "-o", "json")
			if err != nil {
				continue
			}
			var list struct {
				Items []struct {
					Reason        string    `json:"reason"`
					LastTimestamp time.Time `json:"lastTimestamp"`
					EventTime     time.Time `json:"eventTime"`
					Count         int       `json:"count"`
					Metadata      struct {
						CreationTimestamp time.Time `json:"creationTimestamp"`
					} `json:"metadata"`
					Series struct {
						LastObservedTime time.Time `json:"lastObservedTime"`
					} `json:"series"`
				} `json:"items"`
			}
			if json.Unmarshal(raw, &list) != nil {
				continue
			}
			for _, e := range list.Items {
				at := e.LastTimestamp
				if e.Series.LastObservedTime.After(at) {
					at = e.Series.LastObservedTime
				}
				if at.IsZero() {
					at = e.EventTime
				}
				if at.IsZero() {
					at = e.Metadata.CreationTimestamp
				}
				events = append(events, progress.Event{Workload: workload, Reason: progress.Reason(e.Reason), Object: name, At: at, Count: e.Count})
			}
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].At.After(events[j].At) })
	if len(events) > 50 {
		events = events[:50]
	}
	return
}
