// Package stuckpod evaluates StatefulSet Pods that wait on a non-recovering
// container error (UC-08 BR-11, UC-06 CD-11, issue #68).
//
// The default StatefulSet strategy (OrderedReady + RollingUpdate) never
// replaces a current Pod that is not Ready, so a fixed template is never
// applied. Evaluate reports the safe waiting reasons and selects at most one
// Pod of an older revision to delete. It is pure: callers read the objects and
// perform the deletion. Reasons carry only the reason code, the image reference
// and the restart count, never a free-form message.
package stuckpod

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DefaultThreshold is how long a Pod must stay not Ready on a stuck reason
// before it may be replaced.
const DefaultThreshold = 45 * time.Second

// RevisionLabel is the StatefulSet controller's Pod revision label.
const RevisionLabel = "controller-revision-hash"

// stuckReasons do not recover without a new Pod or a template change.
var stuckReasons = map[string]bool{
	"ImagePullBackOff": true, "ErrImagePull": true, "InvalidImageName": true,
	"CrashLoopBackOff": true, "CreateContainerConfigError": true,
	"CreateContainerError": true, "RunContainerError": true,
}

// Reason is one safe observation of a Pod that is not Ready.
type Reason struct {
	Pod       string `json:"pod"`
	Container string `json:"container,omitempty"`
	Code      string `json:"code"`
	Image     string `json:"image,omitempty"`
	Restarts  int64  `json:"restarts,omitempty"`
	// StuckSeconds is how long the Pod has been not Ready.
	StuckSeconds int64 `json:"stuckSeconds"`
}

// String is a short safe summary, e.g. "pg-0: ImagePullBackOff (image postgres:bad)".
func (r Reason) String() string {
	out := r.Pod + ": " + r.Code
	switch {
	case r.Image != "" && (r.Code == "ImagePullBackOff" || r.Code == "ErrImagePull" || r.Code == "InvalidImageName"):
		out += " (image " + r.Image + ")"
	case r.Restarts > 0:
		out += " (restarts " + strconv.FormatInt(r.Restarts, 10) + ")"
	}
	return out
}

// Replacement is the single Pod selected for deletion and why.
type Replacement struct {
	Pod    string `json:"pod"`
	UID    string `json:"-"`
	Reason Reason `json:"reason"`
}

// Decision is the evaluation of one StatefulSet.
type Decision struct {
	// NewRevision is true when updateRevision differs from currentRevision.
	NewRevision bool
	// Reasons lists Pods waiting on a stuck reason, ordered by Pod name.
	Reasons []Reason
	// Replace is set only with a new revision and an eligible Pod.
	Replace *Replacement
}

// Summary joins the reasons for a safe failure or progress message.
func (d Decision) Summary() string {
	parts := make([]string, 0, len(d.Reasons))
	for _, r := range d.Reasons {
		parts = append(parts, r.String())
	}
	return strings.Join(parts, "; ")
}

// Evaluate inspects a StatefulSet and the Pods matched by its selector.
func Evaluate(statefulSet map[string]any, pods []map[string]any, now time.Time, threshold time.Duration) Decision {
	if threshold <= 0 {
		threshold = DefaultThreshold
	}
	status, _ := statefulSet["status"].(map[string]any)
	current, _ := status["currentRevision"].(string)
	update, _ := status["updateRevision"].(string)
	meta, _ := statefulSet["metadata"].(map[string]any)
	setUID, _ := meta["uid"].(string)
	d := Decision{NewRevision: update != "" && current != update}

	type candidate struct {
		ordinal int
		r       Replacement
	}
	var eligible []candidate
	for _, pod := range pods {
		pm, _ := pod["metadata"].(map[string]any)
		name, _ := pm["name"].(string)
		if name == "" || pm["deletionTimestamp"] != nil || !ownedBy(pm, setUID) {
			continue
		}
		ps, _ := pod["status"].(map[string]any)
		ready, since := readyCondition(ps, pm)
		if ready {
			continue
		}
		reason, ok := waitingReason(ps)
		if !ok {
			continue
		}
		reason.Pod = name
		stuck := now.Sub(since)
		if since.IsZero() || stuck < 0 {
			stuck = 0
		}
		reason.StuckSeconds = int64(stuck / time.Second)
		d.Reasons = append(d.Reasons, reason)
		labels, _ := pm["labels"].(map[string]any)
		revision, _ := labels[RevisionLabel].(string)
		if !d.NewRevision || revision == update || since.IsZero() || stuck < threshold {
			continue
		}
		uid, _ := pm["uid"].(string)
		eligible = append(eligible, candidate{ordinal: ordinal(name), r: Replacement{Pod: name, UID: uid, Reason: reason}})
	}
	sort.Slice(d.Reasons, func(i, j int) bool { return d.Reasons[i].Pod < d.Reasons[j].Pod })
	if len(eligible) > 0 {
		// The controller updates from the highest ordinal down; replace that one.
		sort.Slice(eligible, func(i, j int) bool { return eligible[i].ordinal > eligible[j].ordinal })
		r := eligible[0].r
		d.Replace = &r
	}
	return d
}

// ownedBy requires the Pod's controller to be this StatefulSet when its UID is known.
func ownedBy(meta map[string]any, setUID string) bool {
	refs, _ := meta["ownerReferences"].([]any)
	for _, raw := range refs {
		ref, _ := raw.(map[string]any)
		if controller, _ := ref["controller"].(bool); !controller {
			continue
		}
		kind, _ := ref["kind"].(string)
		uid, _ := ref["uid"].(string)
		return kind == "StatefulSet" && (setUID == "" || uid == setUID)
	}
	return false
}

// readyCondition returns the Ready state and since when the Pod is not Ready.
func readyCondition(status, meta map[string]any) (bool, time.Time) {
	conditions, _ := status["conditions"].([]any)
	for _, raw := range conditions {
		c, _ := raw.(map[string]any)
		if c["type"] != "Ready" {
			continue
		}
		if c["status"] == "True" {
			return true, time.Time{}
		}
		if t := parseTime(c["lastTransitionTime"]); !t.IsZero() {
			return false, t
		}
	}
	if t := parseTime(status["startTime"]); !t.IsZero() {
		return false, t
	}
	return false, parseTime(meta["creationTimestamp"])
}

// waitingReason returns the first container (init containers first) waiting on a stuck reason.
func waitingReason(status map[string]any) (Reason, bool) {
	for _, field := range []string{"initContainerStatuses", "containerStatuses"} {
		list, _ := status[field].([]any)
		for _, raw := range list {
			c, _ := raw.(map[string]any)
			state, _ := c["state"].(map[string]any)
			waiting, _ := state["waiting"].(map[string]any)
			code, _ := waiting["reason"].(string)
			if !stuckReasons[code] {
				continue
			}
			name, _ := c["name"].(string)
			image, _ := c["image"].(string)
			restarts, _ := c["restartCount"].(float64)
			return Reason{Container: name, Code: code, Image: safeImage(image), Restarts: int64(restarts)}, true
		}
	}
	return Reason{}, false
}

// safeImage keeps an image reference and drops anything that is not one.
func safeImage(image string) string {
	if len(image) > 256 || strings.ContainsAny(image, " \t\n\"'\\") || strings.Contains(image, "://") {
		return ""
	}
	return image
}

func parseTime(v any) time.Time {
	s, _ := v.(string)
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func ordinal(name string) int {
	i := strings.LastIndex(name, "-")
	if i < 0 {
		return -1
	}
	n, err := strconv.Atoi(name[i+1:])
	if err != nil {
		return -1
	}
	return n
}

// Event is a recorded Pod replacement.
type Event struct {
	Object string    `json:"object"`
	Pod    string    `json:"pod"`
	Reason Reason    `json:"reason"`
	At     time.Time `json:"at"`
}

// String is a safe one-line description of the replacement.
func (e Event) String() string {
	return fmt.Sprintf("%s: replaced %s after %ds", e.Object, e.Reason.String(), e.Reason.StuckSeconds)
}
