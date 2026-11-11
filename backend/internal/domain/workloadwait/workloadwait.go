// Package workloadwait holds the safe, value-free observations of Pods that a
// deployment waits on (UC-08 BR-11, UC-06 CD-11, issue #68).
package workloadwait

import "time"

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
