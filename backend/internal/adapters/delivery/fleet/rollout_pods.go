package fleet

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	k8s "orchestrator/internal/adapters/kubernetes"
	domain "orchestrator/internal/domain/delivery"
)

// UC-06 CD-10: a controller's ready replica count can briefly include a
// container that exits at once. A rollout is Healthy only when every container
// of the current revision's Pods is ready and has been running continuously for
// RolloutStableWindow; a container that keeps restarting is a crash loop.
const (
	RolloutStableWindow  = 15 * time.Second
	CrashLoopRestarts    = 3
	podTemplateHashLabel = "pod-template-hash"
	controllerRevisionLb = "controller-revision-hash"
)

// rolloutPodHealth refines a Healthy or Progressing Deployment/StatefulSet with
// evidence from the Pods of its current revision.
func rolloutPodHealth(ctx context.Context, cli k8s.CLI, namespace, kind string, object map[string]any, now time.Time) (domain.Health, error) {
	selector, ok := matchLabels(object)
	if !ok {
		// Only matchLabels selectors are evaluated; others keep controller health.
		return domain.Healthy, nil
	}
	revision, ok, err := currentPodRevision(ctx, cli, namespace, kind, object, selector)
	if err != nil || !ok {
		return domain.Progressing, err
	}
	pods, err := listItems(ctx, cli, namespace, "pods", selector+","+revision)
	if err != nil {
		return domain.HealthUnknown, err
	}
	if len(pods) == 0 {
		return domain.Progressing, nil
	}
	return podsHealth(pods, now), nil
}

// podsHealth is Degraded when a container that is not stably running has
// restarted CrashLoopRestarts times, Healthy when all are stably running.
func podsHealth(pods []map[string]any, now time.Time) domain.Health {
	health := domain.Healthy
	for _, pod := range pods {
		status, _ := pod["status"].(map[string]any)
		statuses, _ := status["containerStatuses"].([]any)
		if len(statuses) == 0 {
			health = domain.Progressing
			continue
		}
		for _, raw := range statuses {
			c, _ := raw.(map[string]any)
			if stablyRunning(c, now) {
				continue
			}
			restarts, _ := c["restartCount"].(float64)
			if restarts >= CrashLoopRestarts {
				return domain.Degraded
			}
			health = domain.Progressing
		}
	}
	return health
}

func stablyRunning(c map[string]any, now time.Time) bool {
	ready, _ := c["ready"].(bool)
	state, _ := c["state"].(map[string]any)
	running, _ := state["running"].(map[string]any)
	startedAt, _ := running["startedAt"].(string)
	started, err := time.Parse(time.RFC3339, startedAt)
	return ready && err == nil && !now.Before(started.Add(RolloutStableWindow))
}

func matchLabels(object map[string]any) (string, bool) {
	spec, _ := object["spec"].(map[string]any)
	selector, _ := spec["selector"].(map[string]any)
	labels, _ := selector["matchLabels"].(map[string]any)
	if len(labels) == 0 || selector["matchExpressions"] != nil {
		return "", false
	}
	parts := []string{}
	for k, v := range labels {
		value, ok := v.(string)
		if !ok {
			return "", false
		}
		parts = append(parts, k+"="+value)
	}
	sort.Strings(parts)
	return strings.Join(parts, ","), true
}

// currentPodRevision returns the label selecting Pods of the controller's
// current revision, or false while that revision is not yet known.
func currentPodRevision(ctx context.Context, cli k8s.CLI, namespace, kind string, object map[string]any, selector string) (string, bool, error) {
	md, _ := object["metadata"].(map[string]any)
	switch kind {
	case "StatefulSet":
		status, _ := object["status"].(map[string]any)
		revision, _ := status["updateRevision"].(string)
		return controllerRevisionLb + "=" + revision, revision != "", nil
	case "Deployment":
		annotations, _ := md["annotations"].(map[string]any)
		revision, _ := annotations["deployment.kubernetes.io/revision"].(string)
		uid, _ := md["uid"].(string)
		if revision == "" || uid == "" {
			return "", false, nil
		}
		sets, err := listItems(ctx, cli, namespace, "replicasets", selector)
		if err != nil {
			return "", false, err
		}
		for _, set := range sets {
			smd, _ := set["metadata"].(map[string]any)
			a, _ := smd["annotations"].(map[string]any)
			labels, _ := smd["labels"].(map[string]any)
			hash, _ := labels[podTemplateHashLabel].(string)
			if a["deployment.kubernetes.io/revision"] == revision && ownedBy(smd, uid) && hash != "" {
				return podTemplateHashLabel + "=" + hash, true, nil
			}
		}
		return "", false, nil
	}
	return "", false, nil
}

func ownedBy(md map[string]any, uid string) bool {
	owners, _ := md["ownerReferences"].([]any)
	for _, raw := range owners {
		o, _ := raw.(map[string]any)
		if o["uid"] == uid {
			return true
		}
	}
	return false
}

func listItems(ctx context.Context, cli k8s.CLI, namespace, resource, selector string) ([]map[string]any, error) {
	raw, err := cli.Run(ctx, nil, "get", resource, "-n", namespace, "-l", selector, "-o", "json")
	if err != nil {
		return nil, ErrController
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	if json.Unmarshal(raw, &list) != nil {
		return nil, ErrController
	}
	return list.Items, nil
}
