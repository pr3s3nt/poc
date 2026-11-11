package fleet

import (
	"context"
	"encoding/json"
	"time"

	k8s "orchestrator/internal/adapters/kubernetes"
	"orchestrator/internal/adapters/kubernetes/stuckpod"
	"orchestrator/internal/domain/workloadwait"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
)

var _ port.StuckPodRecovery = (*Provider)(nil)

// RecoverStuckPods implements UC-06 CD-11. Fleet only syncs the StatefulSet
// object; its controller never replaces a current Pod that is not Ready. Only
// owned StatefulSets that already carry this delivery revision are considered,
// and a Pod is deleted only when the controller has a new update revision.
// Every query or delete failure (for example a missing permission) only
// reduces what is reported; it never changes the rollout verdict.
func (p *Provider) RecoverStuckPods(ctx context.Context, i port.Intent, r port.Receipt) (port.StuckPods, error) {
	out := port.StuckPods{Waiting: []workloadwait.PodWait{}}
	if p.Manager == nil || p.Manager.Verifier == nil || r.Handle["completionOnly"] == "true" || r.RevisionID != i.Revision.ID {
		return out, nil
	}
	var expected []execution.ObjectRef
	if json.Unmarshal([]byte(r.Handle["inventory"]), &expected) != nil {
		return out, nil
	}
	cli, closeCLI, err := k8s.OpenCLI(ctx, p.Manager.Verifier.KubectlPath, p.Manager.Verifier.Credentials, i.Target)
	if err != nil {
		return out, nil
	}
	defer closeCLI()
	namespace := i.Target.Namespace
	for _, ref := range expected {
		if ref.Kind != "StatefulSet" || !validObjectName(ref.Name) {
			continue
		}
		object, found, err := cli.Get(ctx, namespace, "statefulsets", ref.Name)
		if err != nil || !found {
			continue
		}
		md, _ := object["metadata"].(map[string]any)
		a, _ := md["annotations"].(map[string]any)
		if a["meta.helm.sh/release-name"] != r.Handle["release"] || a["orchestrator.io/delivery-revision"] != i.Revision.ID {
			continue
		}
		selector, ok := matchLabels(object)
		if !ok {
			continue
		}
		pods, err := listItems(ctx, cli, namespace, "pods", selector)
		if err != nil {
			continue
		}
		now := time.Now().UTC()
		decision := stuckpod.Evaluate(object, pods, now, p.StuckThreshold)
		name := namespace + "/" + ref.Name
		for _, reason := range decision.Reasons {
			out.Waiting = append(out.Waiting, wait(name, reason))
		}
		if d := decision.Replace; d != nil && validObjectName(d.Pod) {
			if _, err := cli.Run(ctx, nil, "delete", "pod", d.Pod, "-n", namespace, "--wait=false"); err == nil {
				out.Replaced = append(out.Replaced, workloadwait.PodReplacement{Wait: wait(name, d.Reason), At: now})
			}
		}
	}
	return out, nil
}

func wait(object string, r stuckpod.Reason) workloadwait.PodWait {
	return workloadwait.PodWait{Object: object, Pod: r.Pod, Container: r.Container, Code: r.Code, Image: r.Image, Restarts: r.Restarts, StuckSeconds: r.StuckSeconds}
}
