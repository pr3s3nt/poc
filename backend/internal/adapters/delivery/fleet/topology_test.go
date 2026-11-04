package fleet

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	port "orchestrator/internal/ports/delivery"
	"orchestrator/internal/ports/execution"
)

func topologyWorkload(kind, role, image string) fleetWorkload {
	var w fleetWorkload
	raw, _ := json.Marshal(map[string]any{"kind": kind, "metadata": map[string]any{"name": role, "namespace": "cattle-fleet-system", "uid": role + "-uid", "generation": 2}, "spec": map[string]any{"replicas": 1, "template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{"name": role, "image": image}}}}}, "status": map[string]any{"availableReplicas": 1, "readyReplicas": 1, "updatedReplicas": 1, "observedGeneration": 2}})
	_ = json.Unmarshal(raw, &w)
	return w
}

func TestControllerCompatibilityAndIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, image, label string
		want               bool
	}{
		{"standalone", "rancher/fleet:v0.15.2", "", true},
		{"registry mirror", "registry.example/airgap/fleet:v0.14.1", "", true},
		{"Rancher vendor tag", "rancher/fleet:v0.13.4-rancher.1", "", true},
		{"vendor tag with upstream metadata", "rancher/fleet:v0.13.4-rancher.1", "0.13.4", true},
		{"digest with version metadata", "registry.example/fleet@sha256:abc", "0.15.2", true},
		{"unknown digest", "rancher/fleet@sha256:abc", "", false},
		{"unsupported release", "rancher/fleet:v0.16.0", "", false},
		{"unsupported older release", "rancher/fleet:v0.12.0", "", false},
		{"development version", "rancher/fleet:v0.15.2-rc1", "", false},
		{"conflicting metadata", "rancher/fleet:v0.15.2", "0.14.1", false},
		{"unrelated image", "example/unrelated:v0.15.2", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := topologyWorkload("Deployment", "fleet-controller", tc.image)
			w.Metadata.Labels = map[string]string{"app.kubernetes.io/version": tc.label}
			_, ok := workloadVersion(w, "fleet-controller")
			if ok != tc.want {
				t.Fatalf("supported=%v want=%v", ok, tc.want)
			}
		})
	}
	w := topologyWorkload("Deployment", "fleet-controller", "rancher/fleet:v0.15.2")
	if _, _, err := controllerFrom([]fleetWorkload{w, w}); err == nil {
		t.Fatal("ambiguous managers admitted")
	}
	w.Status.ObservedGeneration = 1
	if _, _, err := controllerFrom([]fleetWorkload{w}); err == nil {
		t.Fatal("stale readiness admitted")
	}
}

func TestOpaqueRegistrationRequiresUniqueAuthenticatedAgentIdentity(t *testing.T) {
	var r fleetRegistration
	r.Metadata.Name = "c-m-abc"
	r.Metadata.Namespace = "fleet-default"
	r.Metadata.Labels = map[string]string{"management.cattle.io/cluster-name": "c-m-abc"}
	r.Spec.ClientID = "opaque-rancher-registration"
	r.Spec.AgentNamespace = "cattle-fleet-system"
	c := workloadCandidate{uid: "physical-uid", clientID: r.Spec.ClientID, agentNamespace: r.Spec.AgentNamespace, agentKind: "StatefulSet", agentName: "fleet-agent", agentVersion: "0.15.2"}
	if _, ok := matchingTarget(r, "manager-uid", "0.15.2", []workloadCandidate{c, c}); !ok {
		t.Fatal("same physical cluster aliases refused")
	}
	other := c
	other.uid = "another-physical-uid"
	if _, ok := matchingTarget(r, "manager-uid", "0.15.2", []workloadCandidate{c, other}); ok {
		t.Fatal("two physical targets claiming one registration admitted")
	}
	other = c
	other.clientID = "unrelated"
	if _, ok := matchingTarget(r, "manager-uid", "0.15.2", []workloadCandidate{other}); ok {
		t.Fatal("display name admitted without registration proof")
	}
	other = c
	other.agentNamespace = "different-agent"
	if _, ok := matchingTarget(r, "manager-uid", "0.15.2", []workloadCandidate{other}); ok {
		t.Fatal("wrong agent namespace admitted")
	}
	other = c
	other.agentVersion = "0.14.1"
	if _, ok := matchingTarget(r, "manager-uid", "0.15.2", []workloadCandidate{other}); ok {
		t.Fatal("controller/agent minor skew admitted")
	}
}

func schemaFixture(paths []string) map[string]any {
	root := map[string]any{}
	for _, path := range paths {
		node := root
		for _, key := range strings.Split(path, ".") {
			props, _ := node["properties"].(map[string]any)
			if props == nil {
				props = map[string]any{}
				node["properties"] = props
			}
			next, _ := props[key].(map[string]any)
			if next == nil {
				next = map[string]any{}
				props[key] = next
			}
			node = next
		}
		types := map[string]string{"spec.paths": "array", "spec.targets": "array", "spec.resources": "array", "status.modifiedStatus": "array", "spec.keepResources": "boolean", "spec.forceSyncGeneration": "integer", "spec.helm": "object", "spec.correctDrift": "object"}
		node["type"] = "string"
		if kind := types[path]; kind != "" {
			node["type"] = kind
		}
	}
	return map[string]any{"spec": map[string]any{"versions": []any{map[string]any{"name": "v1alpha1", "served": true, "schema": map[string]any{"openAPIV3Schema": root}}}}}
}

func TestServedSchemaMustExposeRequiredContract(t *testing.T) {
	for name, paths := range fleetSchemaPaths {
		crd := schemaFixture(paths)
		if !supportsFleetSchema(crd, paths) {
			t.Fatal("complete schema refused", name)
		}
		if supportsFleetSchema(schemaFixture(paths[1:]), paths) {
			t.Fatal("missing required field admitted", name)
		}
		v := crd["spec"].(map[string]any)["versions"].([]any)[0].(map[string]any)
		v["served"] = false
		if supportsFleetSchema(crd, paths) {
			t.Fatal("unserved API admitted", name)
		}
		v["served"] = true
		root := v["schema"].(map[string]any)["openAPIV3Schema"].(map[string]any)
		node := root
		for _, key := range strings.Split(paths[0], ".") {
			node = node["properties"].(map[string]any)[key].(map[string]any)
		}
		node["type"] = "null"
		if supportsFleetSchema(crd, paths) {
			t.Fatal("wrong field type admitted", name)
		}
	}
}

func TestNativeIdentityRequiresExplicitModeAndPreservesPackagePolicy(t *testing.T) {
	config := validConfig()
	config["serviceAccount"] = ""
	if _, err := ParseConfig(config); err == nil {
		t.Fatal("implicit native identity accepted")
	}
	config["serviceAccountMode"] = "NATIVE"
	if _, err := ParseConfig(config); err != nil {
		t.Fatal("explicit native Fleet identity rejected", err)
	}
	i := packageIntent()
	i.Binding.Config = config
	pkg, err := PackageUnit(i, []execution.Manifest{configMap()})
	if err != nil {
		t.Fatal(err)
	}
	var fleet map[string]any
	if yaml.Unmarshal(pkg.Publication.Files["fleet.yaml"], &fleet) != nil {
		t.Fatal("invalid fleet.yaml")
	}
	if fleet["serviceAccount"] != "" || fleet["helm"].(map[string]any)["takeOwnership"] != false {
		t.Fatal("native identity changes ownership policy")
	}
	config["serviceAccount"] = "unexpected-explicit-account"
	if _, err := ParseConfig(config); err == nil {
		t.Fatal("conflicting native/explicit identity accepted")
	}
}

func TestRancherShapedDiscoveryAndVerifyUseOneReadOnlyContract(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLEET_TOPOLOGY_FIXTURE", dir)
	write := func(name string, v any) {
		t.Helper()
		b, _ := json.Marshal(v)
		if err := os.WriteFile(filepath.Join(dir, name+".json"), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	controller := topologyWorkload("Deployment", "fleet-controller", "private.example/mirror/fleet:v0.14.1")
	agent := topologyWorkload("StatefulSet", "fleet-agent", "private.example/mirror/fleet-agent:v0.14.1")
	write("workloads", map[string]any{"items": []fleetWorkload{controller, agent}})
	write("controller", controller)
	write("namespace", map[string]any{"metadata": map[string]any{"uid": "physical-uid"}})
	write("agent", map[string]any{"data": map[string]any{"config": `{"clientID":"rancher-opaque-id"}`}})
	registration := map[string]any{"metadata": map[string]any{"namespace": "fleet-default", "name": "c-abc", "uid": "registration-uid", "labels": map[string]any{"management.cattle.io/cluster-display-name": "Development", "management.cattle.io/cluster-name": "c-abc"}}, "spec": map[string]any{"clientID": "rancher-opaque-id", "agentNamespace": "cattle-fleet-system"}}
	write("registration", registration)
	write("registrations", map[string]any{"items": []any{registration}})
	for name, paths := range fleetSchemaPaths {
		write(name, schemaFixture(paths))
	}
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$FLEET_TOPOLOGY_FIXTURE/calls"
case "$*" in
 *"get deployments,statefulsets "*) cat "$FLEET_TOPOLOGY_FIXTURE/workloads.json";;
 *"get Deployment fleet-controller "*) cat "$FLEET_TOPOLOGY_FIXTURE/controller.json";;
 *"get customresourcedefinitions.apiextensions.k8s.io "*)
   for name in gitrepos bundles bundledeployments clusters; do case "$*" in *" $name.fleet.cattle.io "*) cat "$FLEET_TOPOLOGY_FIXTURE/$name.fleet.cattle.io.json"; exit 0;; esac; done; exit 1;;
 *"get namespace fleet-releases "*|*"get serviceaccount "*) echo 'Error from server (NotFound): not found' >&2;exit 1;;
 *"get namespace "*) cat "$FLEET_TOPOLOGY_FIXTURE/namespace.json";;
 *"get configmap fleet-agent "*) cat "$FLEET_TOPOLOGY_FIXTURE/agent.json";;
 *"get clusters.fleet.cattle.io --all-namespaces "*) cat "$FLEET_TOPOLOGY_FIXTURE/registrations.json";;
 *"get clusters.fleet.cattle.io c-abc "*) cat "$FLEET_TOPOLOGY_FIXTURE/registration.json";;
 *"auth can-i "*) echo yes;;
 *) exit 42;;
esac
`
	executable := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	v := &Verifier{KubectlPath: executable, Credentials: fixtureCredentials{[]byte("private fixture")}, Targets: fixtureTargets{}}
	p := &Provider{Manager: &Manager{Verifier: v}}
	b := port.BindingVersion{ID: "binding", OrganizationID: "org", Version: 1, Config: validConfig()}
	discovery, err := p.InspectConfiguration(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	options := discovery.Options["targetCluster"]
	if len(options) != 1 || options[0].Disabled || options[0].Name != "Development · fleet-default/c-abc" {
		t.Fatalf("Rancher choice unavailable: %+v", options)
	}
	for k, value := range options[0].Values {
		b.Config[k] = value
	}
	if b.Config["serviceAccountMode"] != "NATIVE" || b.Config["releaseNamespace"] != "cattle-fleet-system" || b.Config["targetKubeSystemUID"] != "physical-uid" {
		t.Fatal("discovered topology not pinned")
	}
	b.Config["readSecretMode"] = "MANAGED"
	b.Config["readSecret"] = managedReaderName(b)
	if result, err := v.Verify(context.Background(), b); err != nil || !result.Ready {
		t.Fatal("discovery/Verify disagree", result, err)
	}
	delete(b.Config, "targetKubeSystemUID")
	if _, err := v.Verify(context.Background(), b); err == nil {
		t.Fatal("opaque registration admitted without pinned physical identity")
	}
	b.Config["targetKubeSystemUID"] = "replaced-physical-cluster"
	if _, err := v.Verify(context.Background(), b); err == nil {
		t.Fatal("replaced cluster admitted")
	}
	calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
	for _, line := range strings.Split(string(calls), "\n") {
		if strings.Contains(line, "auth can-i ") {
			continue
		}
		if strings.Contains(line, "get secret ") || strings.Contains(line, " apply ") || strings.Contains(line, " create ") || strings.Contains(line, " patch ") || strings.Contains(line, " delete ") {
			t.Fatal("discovery/Verify read secrets or mutated external state")
		}
	}
}

// Explicit opt-in, read-only probe of an already authorized kind installation.
func TestFleetTopologyKindReadOnly(t *testing.T) {
	path := os.Getenv("ORCH_FLEET_TOPOLOGY_KUBECONFIG")
	if path == "" {
		t.Skip("explicit kind kubeconfig required")
	}
	document, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("private kubeconfig unavailable")
	}
	var config struct {
		CurrentContext string `yaml:"current-context"`
	}
	if yaml.Unmarshal(document, &config) != nil || config.CurrentContext != "kind-k8s-4f" {
		t.Fatal("probe restricted to authorized dev kind context")
	}
	v := &Verifier{Credentials: fixtureCredentials{document}, Targets: fixtureTargets{}}
	p := &Provider{Manager: &Manager{Verifier: v}}
	b := port.BindingVersion{ID: "kind-read-only-probe", Version: 1, OrganizationID: "org", Config: validConfig()}
	discovery, err := p.InspectConfiguration(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range discovery.Options["targetCluster"] {
		if o.Disabled || o.Value != "local" || o.Values["workspace"] != "fleet-local" {
			continue
		}
		for k, v := range o.Values {
			b.Config[k] = v
		}
		found = true
		break
	}
	if !found || b.Config["targetKubeSystemUID"] == "" || b.Config["controllerVersion"] == "" || b.Config["agentKind"] == "" {
		t.Fatal("physical topology not discovered")
	}
	b.Config["readSecretMode"] = "MANAGED"
	b.Config["readSecret"] = managedReaderName(b)
	result, err := v.Verify(context.Background(), b)
	if err != nil || !result.Ready {
		t.Fatal("live kind discovery/Verify disagree", result, err)
	}
	t.Logf("kind read-only discovery/Verify passed: Fleet %s, agent %s/%s; physical UID pinned", b.Config["controllerVersion"], b.Config["agentKind"], b.Config["agentName"])
	// Existing local bindings predate optional topology pins and must remain
	// verifiable through their original bootstrap proof, without rewriting them.
	for _, key := range []string{"controllerKind", "controllerVersion", "agentKind", "agentName", "targetKubeSystemUID"} {
		delete(b.Config, key)
	}
	if result, err := v.Verify(context.Background(), b); err != nil || !result.Ready {
		t.Fatal("legacy local binding rejected", result, err)
	}
	t.Log("legacy local binding verification passed without new topology pins")
}
