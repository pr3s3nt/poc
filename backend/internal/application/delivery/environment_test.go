package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	domain "orchestrator/internal/domain/delivery"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
)

// recordingProvider records every submitted unit payload and can report a
// degraded workload object.
type recordingProvider struct {
	*neutralProvider
	submitted []submittedUnit
	degraded  string
	pruned    []string
}

type submittedUnit struct {
	unit    string
	phase   domain.Phase
	objects []map[string]any
}

func (p *recordingProvider) Submit(ctx context.Context, i port.Intent, prepared port.Prepared) (port.Receipt, error) {
	r, err := p.neutralProvider.Submit(ctx, i, prepared)
	if err != nil {
		return r, err
	}
	var objects []map[string]any
	_ = json.Unmarshal(prepared.Payload, &objects)
	p.submitted = append(p.submitted, submittedUnit{unit: i.Revision.Scope.UnitKey, phase: i.Revision.Phase, objects: objects})
	return r, nil
}

func (p *recordingProvider) Observe(ctx context.Context, i port.Intent, r port.Receipt) (domain.Observation, error) {
	o, err := p.neutralProvider.Observe(ctx, i, r)
	last := p.submitted[len(p.submitted)-1]
	for _, object := range last.objects {
		md := object["metadata"].(map[string]any)
		name := md["name"].(string)
		health := domain.Healthy
		if p.degraded == name && object["kind"] == "Deployment" {
			health = domain.Degraded
			o.Health = domain.Degraded
		}
		o.Objects = append(o.Objects, domain.ObjectObservation{APIVersion: object["apiVersion"].(string), Kind: object["kind"].(string), Name: name, Namespace: "app", UID: "uid-" + name, Health: health})
	}
	return o, err
}

func (p *recordingProvider) PruneReleasedObjects(_ context.Context, _ port.Intent, _ port.Receipt, stale []port.AppliedObject) error {
	for _, o := range stale {
		p.pruned = append(p.pruned, o.Kind+"/"+o.Name)
	}
	return nil
}

func object(apiVersion, kind, name string, extra map[string]any) execution.Manifest {
	obj := map[string]any{"apiVersion": apiVersion, "kind": kind, "metadata": map[string]any{"name": name, "namespace": "app", "labels": map[string]any{"app.kubernetes.io/managed-by": "orchestrator"}}}
	for k, v := range extra {
		obj[k] = v
	}
	return execution.Manifest{APIVersion: apiVersion, Kind: kind, Name: name, Namespace: "app", Object: obj}
}

func workloadManifests(name, image string, secretVersion int, job string) []execution.Manifest {
	out := []execution.Manifest{
		object("secrets.hashicorp.com/v1beta1", "VaultConnection", "orch-vault-1", map[string]any{"spec": map[string]any{"address": "http://vault"}}),
		object("apps/v1", "Deployment", name, map[string]any{"spec": map[string]any{"image": image}}),
	}
	if secretVersion > 0 {
		secret := name + "-config-v" + string(rune('0'+secretVersion))
		out = append(out, object("secrets.hashicorp.com/v1beta1", "VaultStaticSecret", secret, map[string]any{"spec": map[string]any{"version": secretVersion}}))
	}
	if job != "" {
		spec := map[string]any{"args": job}
		if secretVersion > 0 {
			secret := name + "-config-v" + string(rune('0'+secretVersion))
			spec["template"] = map[string]any{"spec": map[string]any{"containers": []any{map[string]any{"name": "main", "env": []any{map[string]any{"name": "PGPASSWORD", "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": secret, "key": "main_PGPASSWORD"}}}}}}}}
		}
		out = append(out, object("batch/v1", "Job", name+"-migrate", map[string]any{"spec": spec}))
	}
	return out
}

func names(objects []map[string]any) []string {
	out := []string{}
	for _, o := range objects {
		md := o["metadata"].(map[string]any)
		out = append(out, o["kind"].(string)+"/"+md["name"].(string))
	}
	sort.Strings(out)
	return out
}

func environmentFixture(t *testing.T) (*Coordinator, *recordingProvider, context.Context, EnvironmentRequest) {
	t.Helper()
	c, neutral, ctx, req, _ := coordinatorFixture(t)
	recording := &recordingProvider{neutralProvider: neutral}
	b, _, err := c.binding(ctx, req.OrganizationKey, req.Environment)
	if err != nil {
		t.Fatal(err)
	}
	registry := port.NewRegistry()
	if err := registry.Register(b.ProviderKind, recording); err != nil {
		t.Fatal(err)
	}
	c.Registry = registry
	return c, recording, ctx, EnvironmentRequest{OrganizationKey: req.OrganizationKey, ApplicationKey: req.ApplicationKey, Environment: req.Environment, Target: req.Target}
}

func TestEnvironmentReleaseMergesWorkloadsAndKeepsLastSuccessfulSecret(t *testing.T) {
	c, p, ctx, req := environmentFixture(t)
	deploy := func(release string, workloads map[string][]execution.Manifest) []string {
		t.Helper()
		req.ReleaseID, req.Workloads = release, workloads
		if err := c.DeployEnvironment(ctx, req); err != nil {
			t.Fatal(err)
		}
		last := p.submitted[len(p.submitted)-1]
		if last.unit != domain.EnvironmentAppUnit || last.phase != domain.Rollout {
			t.Fatalf("last unit = %+v", last)
		}
		return names(last.objects)
	}
	got := deploy("r1", map[string][]execution.Manifest{"backend": workloadManifests("backend", "b:1", 1, ""), "worker": workloadManifests("worker", "w:1", 0, "")})
	want := "Deployment/backend Deployment/worker VaultConnection/orch-vault-1 VaultStaticSecret/backend-config-v1"
	if strings.Join(got, " ") != want {
		t.Fatalf("first release = %v", got)
	}
	// Image-only change: the same secret version, no retained object.
	got = deploy("r2", map[string][]execution.Manifest{"backend": workloadManifests("backend", "b:2", 1, "")})
	if strings.Join(got, " ") != want {
		t.Fatalf("image-only release = %v", got)
	}
	// Secret change: the new version plus the last successful one, marked retained.
	got = deploy("r3", map[string][]execution.Manifest{"backend": workloadManifests("backend", "b:2", 2, "")})
	if strings.Join(got, " ") != "Deployment/backend Deployment/worker VaultConnection/orch-vault-1 VaultStaticSecret/backend-config-v1 VaultStaticSecret/backend-config-v2" {
		t.Fatalf("secret release = %v", got)
	}
	for _, o := range p.submitted[len(p.submitted)-1].objects {
		md := o["metadata"].(map[string]any)
		annotations, _ := md["annotations"].(map[string]any)
		retained := annotations[RetainedSecretAnnotation] == "true"
		if retained != (md["name"] == "backend-config-v1") {
			t.Fatalf("retained marker on %v = %v", md["name"], retained)
		}
	}
	// The next successful release no longer needs v1.
	got = deploy("r4", map[string][]execution.Manifest{"backend": workloadManifests("backend", "b:3", 2, "")})
	if strings.Join(got, " ") != "Deployment/backend Deployment/worker VaultConnection/orch-vault-1 VaultStaticSecret/backend-config-v2" {
		t.Fatalf("release after success = %v", got)
	}
	// Removing a workload drops it from the bundle.
	req.Removed = []string{"worker"}
	got = deploy("r5", map[string][]execution.Manifest{})
	req.Removed = nil
	if strings.Join(got, " ") != "Deployment/backend VaultConnection/orch-vault-1 VaultStaticSecret/backend-config-v2" {
		t.Fatalf("removal release = %v", got)
	}
}

func TestEnvironmentReleaseRunsChangedJobsFirstAndOnlyOnce(t *testing.T) {
	c, p, ctx, req := environmentFixture(t)
	req.ReleaseID, req.Workloads = "r1", map[string][]execution.Manifest{"backend": workloadManifests("backend", "b:1", 0, "migrate")}
	if err := c.DeployEnvironment(ctx, req); err != nil {
		t.Fatal(err)
	}
	if len(p.submitted) != 2 || p.submitted[0].unit != domain.EnvironmentMigrateUnit || p.submitted[0].phase != domain.Job || p.submitted[1].unit != domain.EnvironmentAppUnit {
		t.Fatalf("order = %+v", p.submitted)
	}
	jobs := names(p.submitted[0].objects)
	if len(jobs) != 1 || !strings.HasPrefix(jobs[0], "Job/backend-migrate-") || len(jobs[0]) != len("Job/backend-migrate-")+8 {
		t.Fatalf("job = %v", jobs)
	}
	for _, name := range names(p.submitted[1].objects) {
		if strings.HasPrefix(name, "Job/") {
			t.Fatal("Job in app unit")
		}
	}
	// Same Job content, new image: the migrate unit is not published again.
	req.ReleaseID, req.Workloads = "r2", map[string][]execution.Manifest{"backend": workloadManifests("backend", "b:2", 0, "migrate")}
	if err := c.DeployEnvironment(ctx, req); err != nil {
		t.Fatal(err)
	}
	if len(p.submitted) != 3 || p.submitted[2].unit != domain.EnvironmentAppUnit {
		t.Fatalf("unchanged Job republished: %+v", p.submitted)
	}
	// A changed Job is a new object with a new name.
	req.ReleaseID, req.Workloads = "r3", map[string][]execution.Manifest{"backend": workloadManifests("backend", "b:2", 0, "migrate-2")}
	if err := c.DeployEnvironment(ctx, req); err != nil {
		t.Fatal(err)
	}
	if len(p.submitted) != 5 || p.submitted[3].unit != domain.EnvironmentMigrateUnit || names(p.submitted[3].objects)[0] == jobs[0] {
		t.Fatalf("changed Job = %+v", p.submitted[3:])
	}
}

func TestEnvironmentReleaseNamesTheFailingWorkload(t *testing.T) {
	c, p, ctx, req := environmentFixture(t)
	c.Deadline = time.Second
	p.degraded = "worker"
	req.ReleaseID, req.Workloads = "r1", map[string][]execution.Manifest{"backend": workloadManifests("backend", "b:1", 0, ""), "worker": workloadManifests("worker", "w:bad", 1, "")}
	err := c.DeployEnvironment(ctx, req)
	var failure *RolloutFailure
	if !errors.As(err, &failure) || !errors.Is(err, ErrRolloutDegraded) || strings.Join(failure.Workloads, ",") != "worker" {
		t.Fatalf("failure = %#v (%v)", failure, err)
	}
	// The next release starts from the published (failed) state.
	p.degraded = ""
	req.ReleaseID, req.Workloads = "r2", map[string][]execution.Manifest{"worker": workloadManifests("worker", "w:good", 0, "")}
	if err := c.DeployEnvironment(ctx, req); err != nil {
		t.Fatal(err)
	}
	if strings.Join(names(p.submitted[len(p.submitted)-1].objects), " ") != "Deployment/backend Deployment/worker VaultConnection/orch-vault-1" {
		t.Fatalf("fix release = %v", names(p.submitted[len(p.submitted)-1].objects))
	}
	// The secret only the failed revision created is pruned once the fix is Ready.
	if strings.Join(p.pruned, ",") != "VaultStaticSecret/worker-config-v1" {
		t.Fatalf("pruned = %v", p.pruned)
	}
}

func TestEnvironmentBundleIsNotUsedWhileLegacyUnitsAreActive(t *testing.T) {
	c, _, ctx, req, _ := coordinatorFixture(t)
	uses, err := c.UsesEnvironmentBundle(ctx, req.OrganizationKey, req.Environment)
	if err != nil || !uses {
		t.Fatalf("fresh Environment = %v %v", uses, err)
	}
	if err := c.Deploy(ctx, req); err != nil {
		t.Fatal(err)
	}
	uses, err = c.UsesEnvironmentBundle(ctx, req.OrganizationKey, req.Environment)
	if err != nil || uses {
		t.Fatalf("legacy Environment = %v %v", uses, err)
	}
}

func TestEnvironmentReleasePublishesMissingJobSecretsBeforeTheJob(t *testing.T) {
	c, p, ctx, req := environmentFixture(t)
	req.ReleaseID, req.Workloads = "r1", map[string][]execution.Manifest{"backend": workloadManifests("backend", "b:1", 1, "migrate"), "worker": workloadManifests("worker", "w:1", 0, "")}
	if err := c.DeployEnvironment(ctx, req); err != nil {
		t.Fatal(err)
	}
	if len(p.submitted) != 3 || p.submitted[0].unit != domain.EnvironmentAppUnit || p.submitted[1].unit != domain.EnvironmentMigrateUnit || p.submitted[2].unit != domain.EnvironmentAppUnit {
		t.Fatalf("order = %+v", p.submitted)
	}
	if got := strings.Join(names(p.submitted[0].objects), " "); got != "VaultConnection/orch-vault-1 VaultStaticSecret/backend-config-v1" {
		t.Fatalf("secret preparation = %s", got)
	}
	if got := strings.Join(names(p.submitted[2].objects), " "); got != "Deployment/backend Deployment/worker VaultConnection/orch-vault-1 VaultStaticSecret/backend-config-v1" {
		t.Fatalf("final app = %s", got)
	}
	// A new secret version for a changed Job is prepared again; the running
	// version stays declared until the final release.
	req.ReleaseID, req.Workloads = "r2", map[string][]execution.Manifest{"backend": workloadManifests("backend", "b:1", 2, "migrate")}
	if err := c.DeployEnvironment(ctx, req); err != nil {
		t.Fatal(err)
	}
	if len(p.submitted) != 6 || p.submitted[3].unit != domain.EnvironmentAppUnit || p.submitted[4].unit != domain.EnvironmentMigrateUnit {
		t.Fatalf("order = %+v", p.submitted[3:])
	}
	if got := strings.Join(names(p.submitted[3].objects), " "); got != "Deployment/backend Deployment/worker VaultConnection/orch-vault-1 VaultStaticSecret/backend-config-v1 VaultStaticSecret/backend-config-v2" {
		t.Fatalf("second preparation = %s", got)
	}
	if got := strings.Join(names(p.submitted[5].objects), " "); got != "Deployment/backend Deployment/worker VaultConnection/orch-vault-1 VaultStaticSecret/backend-config-v1 VaultStaticSecret/backend-config-v2" {
		t.Fatalf("final with retained secret = %s", got)
	}
}
