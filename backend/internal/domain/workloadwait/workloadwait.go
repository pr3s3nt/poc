// Package workloadwait holds the safe, value-free observations of Pods that a
// deployment waits on (UC-08 BR-11, UC-06 CD-11, issue #68).
package workloadwait

import (
	"strings"
	"time"
)

// PodWait is one Pod that is not Ready: reason code, image reference and
// restarts only, never a free-form message.
type PodWait struct {
	Object       string `json:"object"`
	Pod          string `json:"pod"`
	Container    string `json:"container,omitempty"`
	Code         string `json:"code"`
	Image        string `json:"image,omitempty"`
	Restarts     int64  `json:"restarts,omitempty"`
	StuckSeconds int64  `json:"stuckSeconds"`
}

// PodReplacement records that a stuck Pod of an older revision was deleted so
// the StatefulSet controller recreates it from the fixed revision.
type PodReplacement struct {
	Wait PodWait   `json:"wait"`
	At   time.Time `json:"at"`
}

// Detail is the persisted wait state of one resource or workload, kept in the
// owning Environment operation detail.
type Detail struct {
	DeploymentID string           `json:"deploymentId,omitempty"`
	Waiting      []PodWait        `json:"waiting"`
	Replaced     []PodReplacement `json:"replaced,omitempty"`
}

// stuckReasons do not recover without a new Pod or a template change.
var stuckReasons = map[string]bool{
	"ImagePullBackOff": true, "ErrImagePull": true, "InvalidImageName": true,
	"CrashLoopBackOff": true, "CreateContainerConfigError": true,
	"CreateContainerError": true, "RunContainerError": true,
}

// StuckReason reports whether a container waiting reason is non-recovering.
func StuckReason(code string) bool { return stuckReasons[code] }

// SafeImage keeps an image reference and drops anything that is not one.
func SafeImage(image string) string {
	if len(image) > 256 || strings.ContainsAny(image, " \t\n\"'\\<>") || strings.Contains(image, "://") {
		return ""
	}
	return image
}

// SafeName accepts a Kubernetes object name, optionally "namespace/name".
func SafeName(s string) bool {
	if s == "" || len(s) > 507 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '.' || c == '/') {
			return false
		}
	}
	return true
}

// Sanitize re-validates a persisted detail before it is shown: only
// allowlisted reason codes, safe names and image references remain.
func Sanitize(d Detail) Detail {
	keep := func(w PodWait) (PodWait, bool) {
		if !StuckReason(w.Code) || !SafeName(w.Pod) || !SafeName(w.Object) || (w.Container != "" && !SafeName(w.Container)) {
			return PodWait{}, false
		}
		w.Image = SafeImage(w.Image)
		return w, true
	}
	out := Detail{DeploymentID: "", Waiting: []PodWait{}}
	for _, w := range d.Waiting {
		if w, ok := keep(w); ok {
			out.Waiting = append(out.Waiting, w)
		}
	}
	for _, r := range d.Replaced {
		if w, ok := keep(r.Wait); ok {
			out.Replaced = append(out.Replaced, PodReplacement{Wait: w, At: r.At})
		}
	}
	return out
}
