package v04run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"orchestrator/test/integration/v03run"
	"orchestrator/test/integration/v2run"
)

// Expect is what a verified acceptance application must look like in the namespace.
type Expect struct {
	Namespace string
	// Images maps workload -> expected image reference.
	Images map[string]string
	// Digests optionally maps workload -> expected sha256 of the image running in every pod.
	Digests map[string]string
}

// Identity is the set of object UIDs that prove a run reused (did not recreate) its shared
// services. UIDs are Kubernetes object identities, not secrets.
type Identity struct {
	NamespaceUID string   `json:"namespaceUid"`
	PostgresUID  string   `json:"postgresUid"`
	RedisUID     string   `json:"redisUid"`
	PVCs         []string `json:"pvcUids"`
	PVs          []string `json:"pvNames"`
}

// AppReport is the safe verdict of one verification. It holds names, counts and booleans only.
type AppReport struct {
	Workloads       map[string]v03run.Report `json:"workloads"`
	SecretBindings  map[string][]string      `json:"secretBindings"`
	SharedHosts     map[string]bool          `json:"sharedHosts"`
	PostgresSets    int                      `json:"postgresStatefulSets"`
	RedisSets       int                      `json:"redisStatefulSets"`
	Identity        Identity                 `json:"identity"`
	PlainLiteralFor []string                 `json:"passwordsAsPlainEnv"`
}

type envEntry struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	ValueFrom *struct {
		SecretKeyRef *struct {
			Name string `json:"name"`
			Key  string `json:"key"`
		} `json:"secretKeyRef"`
	} `json:"valueFrom"`
}

func deploymentEnv(ctx context.Context, k v2run.Kubectl, namespace, name string) ([]envEntry, error) {
	raw, stderr, exit := k.Run(ctx, "-n", namespace, "get", "deployment", name, "-o", "json")
	if exit != 0 {
		return nil, fmt.Errorf("deployment %s is missing: %s", name, firstLine(stderr))
	}
	var doc struct {
		Spec struct {
			Template struct {
				Spec struct {
					Containers []struct {
						Env []envEntry `json:"env"`
					} `json:"containers"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
	}
	if json.Unmarshal(raw, &doc) != nil || len(doc.Spec.Template.Spec.Containers) == 0 {
		return nil, errors.New("the deployment cannot be read")
	}
	return doc.Spec.Template.Spec.Containers[0].Env, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}

// secretNames are the variables that must reach a Pod only through a Kubernetes Secret reference.
var secretNames = map[string][]string{
	"backend": {"PGPASSWORD", "REDIS_PASSWORD", "ACCEPTANCE_SECRET"},
	"worker":  {"PGPASSWORD", "REDIS_PASSWORD"},
}

// plainShared are non-secret bindings that must exist; backend and worker must agree on their VALUES for the
// shared services (the host names are Service names, not secrets).
var plainShared = []string{"PGHOST", "PGPORT", "PGDATABASE", "PGUSER", "REDIS_HOST", "REDIS_PORT", "REDIS_USERNAME", "REDIS_TLS", "ACCEPTANCE_CACHE_RUN", "ACCEPTANCE_CACHE_TTL_SECONDS"}

type workloadSet struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
			UID  string `json:"uid"`
		} `json:"metadata"`
		Spec struct {
			Template struct {
				Spec struct {
					Containers []struct {
						Image string `json:"image"`
					} `json:"containers"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
	} `json:"items"`
}

// VerifyApp checks the whole application in one namespace: three fully rolled-out Deployments with the
// expected images, secrets only through Secret references, ONE PostgreSQL and ONE Redis shared by backend
// and worker, and their object identities. An error names the failed property, never a value.
func VerifyApp(ctx context.Context, k v2run.Kubectl, e Expect) (AppReport, error) {
	report := AppReport{Workloads: map[string]v03run.Report{}, SecretBindings: map[string][]string{}, SharedHosts: map[string]bool{}}
	for _, name := range []string{"backend", "worker", "frontend"} {
		w, err := v03run.VerifyWorkload(ctx, k, e.Namespace, name, e.Images[name], e.Digests[name], nil, true)
		report.Workloads[name] = w
		if err != nil {
			return report, fmt.Errorf("%s: %w", name, err)
		}
	}
	envs := map[string]map[string]envEntry{}
	for _, name := range []string{"backend", "worker"} {
		entries, err := deploymentEnv(ctx, k, e.Namespace, name)
		if err != nil {
			return report, err
		}
		envs[name] = map[string]envEntry{}
		for _, entry := range entries {
			envs[name][entry.Name] = entry
		}
		for _, secret := range secretNames[name] {
			entry, ok := envs[name][secret]
			switch {
			case !ok:
				return report, fmt.Errorf("%s: %s is not bound", name, secret)
			case entry.Value != "" || entry.ValueFrom == nil || entry.ValueFrom.SecretKeyRef == nil:
				report.PlainLiteralFor = append(report.PlainLiteralFor, name+"/"+secret)
				return report, fmt.Errorf("%s: %s must come from a Secret reference, not a plain value", name, secret)
			}
			report.SecretBindings[name] = append(report.SecretBindings[name], secret)
		}
		for _, plain := range plainShared {
			if _, ok := envs[name][plain]; !ok {
				return report, fmt.Errorf("%s: %s is not bound", name, plain)
			}
		}
	}
	for _, key := range plainShared {
		same := envs["backend"][key].Value == envs["worker"][key].Value && envs["backend"][key].Value != ""
		report.SharedHosts[key] = same
		if (strings.HasPrefix(key, "PG") || strings.HasPrefix(key, "REDIS")) && !same {
			return report, fmt.Errorf("backend and worker bind different %s: they must share one service", key)
		}
	}

	raw, stderr, exit := k.Run(ctx, "-n", e.Namespace, "get", "statefulsets", "-o", "json")
	if exit != 0 {
		return report, fmt.Errorf("statefulsets cannot be listed: %s", firstLine(stderr))
	}
	var sets workloadSet
	if json.Unmarshal(raw, &sets) != nil {
		return report, errors.New("the StatefulSet list cannot be read")
	}
	for _, item := range sets.Items {
		for _, container := range item.Spec.Template.Spec.Containers {
			switch {
			case strings.Contains(container.Image, "postgres"):
				report.PostgresSets++
				report.Identity.PostgresUID = item.Metadata.UID
			case strings.Contains(container.Image, "redis"):
				report.RedisSets++
				report.Identity.RedisUID = item.Metadata.UID
			}
		}
	}
	if report.PostgresSets != 1 || report.RedisSets != 1 {
		return report, fmt.Errorf("expected exactly one PostgreSQL and one Redis StatefulSet, found %d and %d", report.PostgresSets, report.RedisSets)
	}
	identity, err := ReadIdentity(ctx, k, e.Namespace)
	if err != nil {
		return report, err
	}
	identity.PostgresUID, identity.RedisUID = report.Identity.PostgresUID, report.Identity.RedisUID
	report.Identity = identity
	if len(identity.PVCs) < 2 {
		return report, fmt.Errorf("expected a PersistentVolumeClaim for PostgreSQL and for Redis, found %d", len(identity.PVCs))
	}
	return report, nil
}

// ReadIdentity returns the namespace UID, the claimed PersistentVolumeClaim UIDs and the PersistentVolumes bound to them.
func ReadIdentity(ctx context.Context, k v2run.Kubectl, namespace string) (Identity, error) {
	var out Identity
	raw, stderr, exit := k.Run(ctx, "get", "namespace", namespace, "-o", "jsonpath={.metadata.uid}")
	if exit != 0 {
		return out, fmt.Errorf("namespace %s cannot be read: %s", namespace, firstLine(stderr))
	}
	out.NamespaceUID = strings.TrimSpace(string(raw))
	raw, stderr, exit = k.Run(ctx, "-n", namespace, "get", "pvc", "-o", "json")
	if exit != 0 {
		return out, fmt.Errorf("claims cannot be listed: %s", firstLine(stderr))
	}
	var pvcs struct {
		Items []struct {
			Metadata struct {
				UID string `json:"uid"`
			} `json:"metadata"`
			Spec struct {
				VolumeName string `json:"volumeName"`
			} `json:"spec"`
		} `json:"items"`
	}
	if json.Unmarshal(raw, &pvcs) != nil {
		return out, errors.New("the claim list cannot be read")
	}
	for _, item := range pvcs.Items {
		out.PVCs = append(out.PVCs, item.Metadata.UID)
		if item.Spec.VolumeName != "" {
			out.PVs = append(out.PVs, item.Spec.VolumeName)
		}
	}
	sort.Strings(out.PVCs)
	sort.Strings(out.PVs)
	return out, nil
}

// SameIdentity reports whether two snapshots describe the very same shared services (no recreation).
func SameIdentity(a, b Identity) bool {
	return a.NamespaceUID == b.NamespaceUID && a.PostgresUID == b.PostgresUID && a.RedisUID == b.RedisUID &&
		strings.Join(a.PVCs, ",") == strings.Join(b.PVCs, ",") && strings.Join(a.PVs, ",") == strings.Join(b.PVs, ",")
}

// VerifyVolumesGone proves none of the recorded PersistentVolumes remains (run storage is cleaned with its namespace).
func VerifyVolumesGone(ctx context.Context, k v2run.Kubectl, volumes []string) error {
	for _, name := range volumes {
		_, stderr, exit := k.Run(ctx, "get", "pv", name, "-o", "name")
		switch {
		case exit == 0:
			return fmt.Errorf("persistent volume %s still exists", name)
		case !strings.Contains(stderr, "NotFound") && !strings.Contains(strings.ToLower(stderr), "not found"):
			// An API error is not proof of absence.
			return fmt.Errorf("persistent volume %s absence cannot be proven: %s", name, firstLine(stderr))
		}
	}
	return nil
}
