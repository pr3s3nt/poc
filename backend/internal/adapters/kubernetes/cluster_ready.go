package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"orchestrator/internal/ports/execution"
)

// ClusterReady implements execution.ClusterReadiness over kubectl and the scoped target credentials
// (ADR-017). Cluster ACTIVE is never enough: each attempt derives FRESH short-lived credentials for the
// target's original account, then proves API reachability, authentication, authorisation for the objects
// consumers create and at least one Ready, schedulable node with allocatable capacity.
type ClusterReady struct {
	KubectlPath string
	Credentials execution.KubeconfigSource
	// Interval is the pause between attempts (default 10s).
	Interval time.Duration
	// Sleep is the pause seam (tests).
	Sleep func(ctx context.Context, d time.Duration)
}

var _ execution.ClusterReadiness = (*ClusterReady)(nil)

// consumerObjects are the objects the namespace, PostgreSQL/Redis modules and workloads create.
var consumerObjects = [][]string{{"namespaces"}, {"deployments", "-A"}, {"statefulsets", "-A"}, {"services", "-A"}, {"secrets", "-A"}}

// WaitClusterReady implements execution.ClusterReadiness.
func (c *ClusterReady) WaitClusterReady(ctx context.Context, target execution.Target) error {
	if !target.Explicit() {
		return errors.New("kubernetes: an explicit cluster target is required")
	}
	interval := c.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	// last is the most recent category observed while the deadline had not passed; an attempt cut short by the
	// deadline says nothing about the cluster and never replaces it.
	last := execution.ErrClusterNotReady
	for {
		err := c.attempt(ctx, target)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return last
		}
		last = err
		// An authorisation answer from the API is deterministic: waiting does not fix it.
		if errors.Is(last, execution.ErrClusterForbidden) {
			return last
		}
		c.sleep(ctx, interval)
		if ctx.Err() != nil {
			return last
		}
	}
}

func (c *ClusterReady) sleep(ctx context.Context, d time.Duration) {
	if c.Sleep != nil {
		c.Sleep(ctx, d)
		return
	}
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

// attempt performs one complete check with credentials derived for this attempt only.
func (c *ClusterReady) attempt(ctx context.Context, target execution.Target) error {
	cli, cleanup, err := OpenCLI(ctx, c.KubectlPath, c.Credentials, target)
	if err != nil {
		// The credential could not be derived: a cluster that is not yet answering EKS describe calls looks like this.
		return execution.ErrClusterAuth
	}
	defer cleanup()
	if _, err := cli.Run(ctx, nil, "version", "-o", "json"); err != nil {
		return categorize(err)
	}
	for _, object := range consumerObjects {
		out, err := cli.Run(ctx, nil, append([]string{"auth", "can-i", "create"}, object...)...)
		if err != nil {
			// `can-i` exits non-zero with "no"; an authentication failure still classifies as such.
			if errors.Is(err, ErrUnauthorized) {
				return execution.ErrClusterAuth
			}
			if errors.Is(err, ErrUnreachable) {
				return execution.ErrClusterUnreachable
			}
			if strings.TrimSpace(string(out)) == "no" {
				return execution.ErrClusterForbidden
			}
			return categorize(err)
		}
		if strings.TrimSpace(string(out)) != "yes" {
			return execution.ErrClusterForbidden
		}
	}
	raw, err := cli.Run(ctx, nil, "get", "nodes", "-o", "json")
	if err != nil {
		return categorize(err)
	}
	if !usableNodeCapacity(raw) {
		return execution.ErrClusterNoCapacity
	}
	return nil
}

func categorize(err error) error {
	switch {
	case errors.Is(err, ErrUnauthorized):
		return execution.ErrClusterAuth
	case errors.Is(err, ErrForbidden):
		return execution.ErrClusterForbidden
	case errors.Is(err, ErrUnreachable):
		return execution.ErrClusterUnreachable
	}
	return execution.ErrClusterNotReady
}

// usableNodeCapacity reports whether at least one node is Ready, schedulable (not cordoned, no NoSchedule or
// NoExecute taint) and has allocatable CPU, memory and pod capacity.
func usableNodeCapacity(raw []byte) bool {
	var list struct {
		Items []struct {
			Spec struct {
				Unschedulable bool `json:"unschedulable"`
				Taints        []struct {
					Effect string `json:"effect"`
				} `json:"taints"`
			} `json:"spec"`
			Status struct {
				Conditions  []struct{ Type, Status string } `json:"conditions"`
				Allocatable map[string]string               `json:"allocatable"`
			} `json:"status"`
		} `json:"items"`
	}
	if json.Unmarshal(raw, &list) != nil {
		return false
	}
	for _, node := range list.Items {
		ready := false
		for _, condition := range node.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == "True" {
				ready = true
			}
		}
		if !ready || node.Spec.Unschedulable {
			continue
		}
		blocked := false
		for _, taint := range node.Spec.Taints {
			if taint.Effect == "NoSchedule" || taint.Effect == "NoExecute" {
				blocked = true
			}
		}
		if blocked {
			continue
		}
		alloc := node.Status.Allocatable
		if positive(alloc["cpu"]) && positive(alloc["memory"]) && positive(alloc["pods"]) {
			return true
		}
	}
	return false
}

// positive accepts a Kubernetes quantity that is not zero (for example "2", "1930m", "3Gi").
func positive(quantity string) bool {
	quantity = strings.TrimSpace(quantity)
	if quantity == "" {
		return false
	}
	for _, r := range quantity {
		if r >= '1' && r <= '9' {
			return true
		}
	}
	return false
}
